package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/silaselisha/coffee-api/pkg/store"
	"github.com/stretchr/testify/require"
)

func TestCreateProduct(t *testing.T) {

	var tests = []struct {
		name       string
		bodyWriter func() (*bytes.Buffer, *multipart.Writer)
		check      func(t *testing.T, recorder *httptest.ResponseRecorder)
	}{
		{
			name: "create a product",
			bodyWriter: func() (*bytes.Buffer, *multipart.Writer) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				writer.WriteField("name", product.Name)
				writer.WriteField("price", strconv.FormatFloat(product.Price, 'E', -1, 64))
				writer.WriteField("summary", product.Summary)
				writer.WriteField("category", product.Category)
				writer.WriteField("description", product.Description)
				writer.WriteField("ingridients", strings.Join(product.Ingridients, " "))

				palette := []color.Color{color.Black, color.White}

				w, err := writer.CreateFormFile("thumbnail", "thumbnail.jpeg")
				require.NoError(t, err)
				img := image.NewPaletted(image.Rect(0, 0, 800, 400), palette)
				err = png.Encode(w, img)
				require.NoError(t, err)

				writer.WriteField("created_at", fmt.Sprint(product.CreatedAt))
				writer.WriteField("updated_at", fmt.Sprint(product.UpdatedAt))
				defer writer.Close()
				return body, writer
			},
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				dataBytes, err := io.ReadAll(recorder.Body)
				require.Equal(t, nil, err)
				var result struct {
					Status string
					Data   store.Item
				}
				err = json.Unmarshal(dataBytes, &result)
				require.Equal(t, nil, err)
				productID = result.Data.Id.Hex()

				fmt.Println(productID)
				require.NotEmpty(t, productID)
				require.Equal(t, http.StatusCreated, recorder.Code)
			},
		},
		{
			name: "create a duplicate product",
			bodyWriter: func() (*bytes.Buffer, *multipart.Writer) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				writer.WriteField("name", product.Name)
				writer.WriteField("price", strconv.FormatFloat(product.Price, 'E', -1, 64))
				writer.WriteField("summary", product.Summary)
				writer.WriteField("category", product.Category)
				writer.WriteField("description", product.Description)
				writer.WriteField("ingridients", strings.Join(product.Ingridients, " "))

				palette := []color.Color{color.Black, color.White}

				w, err := writer.CreateFormFile("thumbnail", "thumbnail.jpeg")
				require.NoError(t, err)
				img := image.NewPaletted(image.Rect(0, 0, 800, 400), palette)
				err = png.Encode(w, img)
				require.NoError(t, err)

				writer.WriteField("created_at", fmt.Sprint(product.CreatedAt))
				writer.WriteField("updated_at", fmt.Sprint(product.UpdatedAt))
				defer writer.Close()
				return body, writer
			},
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusInternalServerError, recorder.Code)
			},
		},
		{
			name: "create a product without thumbnail",
			bodyWriter: func() (*bytes.Buffer, *multipart.Writer) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				writer.WriteField("name", product.Name)
				writer.WriteField("price", strconv.FormatFloat(product.Price, 'E', -1, 64))
				writer.WriteField("summary", product.Summary)
				writer.WriteField("category", product.Category)
				writer.WriteField("description", product.Description)
				writer.WriteField("ingridients", strings.Join(product.Ingridients, " "))

				writer.WriteField("created_at", fmt.Sprint(product.CreatedAt))
				writer.WriteField("updated_at", fmt.Sprint(product.UpdatedAt))
				defer writer.Close()
				return body, writer
			},
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusBadRequest, recorder.Code)
			},
		},
		{
			name: "create an invalid product",
			bodyWriter: func() (*bytes.Buffer, *multipart.Writer) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				defer writer.Close()
				return body, writer
			},
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusBadRequest, recorder.Code)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server := NewServer(ctx, mongoClient, distributor)

			url := "/products"
			recorder := httptest.NewRecorder()
			body, writer := test.bodyWriter()
			request, err := http.NewRequest(http.MethodPost, url, body)
			require.NoError(t, err)
			request.Header.Set("Content-Type", "multipart/form-data; boundary="+writer.Boundary())

			mux, ok := server.(*Server)
			require.Equal(t, true, ok)

			mux.Router.ServeHTTP(recorder, request)
			test.check(t, recorder)
		})
	}
}

func TestUpdateProduct(t *testing.T) {
	var tests = []struct {
		name       string
		id         string
		bodyWriter func() (*bytes.Buffer, *multipart.Writer)
		check      func(t *testing.T, recorder *httptest.ResponseRecorder)
	}{
		{
			name: "update a product",
			id:   productID,
			bodyWriter: func() (*bytes.Buffer, *multipart.Writer) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				writer.WriteField("price", "4.99")
				defer writer.Close()
				return body, writer
			},
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, recorder.Code)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			url := fmt.Sprintf("/products/%s", test.id)
			recorder := httptest.NewRecorder()
			body, writer := test.bodyWriter()
			request, err := http.NewRequest(http.MethodPut, url, body)
			require.NoError(t, err)
			request.Header.Set("Content-Type", "multipart/form-data; boundary="+writer.Boundary())

			querier := NewServer(ctx, mongoClient, distributor)
			server, ok := querier.(*Server)
			require.Equal(t, true, ok)

			server.Router.ServeHTTP(recorder, request)
			test.check(t, recorder)
		})
	}
}

func TestGetAllProduct(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T, recorder *httptest.ResponseRecorder)
	}{
		{
			name: "get all products",
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, recorder.Code)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server := NewServer(ctx, mongoClient, distributor)

			url := "/products"
			recorder := httptest.NewRecorder()
			request, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)

			mux, ok := server.(*Server)
			require.Equal(t, true, ok)

			mux.Router.ServeHTTP(recorder, request)
			test.check(t, recorder)
		})
	}
}

func TestGetProduct(t *testing.T) {
	tests := []struct {
		name     string
		category map[string]interface{}
		id       string
		check    func(t *testing.T, recorder *httptest.ResponseRecorder)
	}{
		{
			name:     "get product by category & id",
			category: map[string]interface{}{"category": "beverages"},
			id:       productID,
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, recorder.Code)
			},
		},
		{
			name:     "get product by invalid id",
			category: map[string]interface{}{"category": "beverages"},
			id:       "65bcc06cbc92379c5b6fe79b",
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusNotFound, recorder.Code)
			},
		},
		{
			name:     "get product by invalid category",
			category: map[string]interface{}{"category": "beverage"},
			id:       productID,
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusNotFound, recorder.Code)
			},
		},
		{
			name:     "get product by invalid mongo id",
			category: map[string]interface{}{"category": "beverage"},
			id:       "65bcc06cbc9",
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusBadRequest, recorder.Code)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server := NewServer(ctx, mongoClient, distributor)

			url := fmt.Sprintf("/products/%s/%s", test.category["category"], test.id)
			recorder := httptest.NewRecorder()
			request, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)

			mux, ok := server.(*Server)
			require.Equal(t, true, ok)

			mux.Router.ServeHTTP(recorder, request)
			test.check(t, recorder)
		})
	}
}

func TestDeleteProduct(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		check func(t *testing.T, recorder *httptest.ResponseRecorder)
	}{
		{
			name: "delete product by id",
			id:   productID,
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusNoContent, recorder.Code)
			},
		},
		{
			name: "delete product by invalid id",
			id:   "65bcc06cbc92379c5b6fe79b",
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusBadRequest, recorder.Code)
			},
		},
		{
			name: "delete product by invalid mongo id",
			id:   "65bcc06cbc92379c5b6fe79bcd56",
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusBadRequest, recorder.Code)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server := NewServer(ctx, mongoClient, distributor)

			url := fmt.Sprintf("/products/%s", test.id)
			fmt.Println(url)
			recorder := httptest.NewRecorder()
			request, err := http.NewRequest(http.MethodDelete, url, nil)
			require.NoError(t, err)

			mux, ok := server.(*Server)
			require.Equal(t, true, ok)

			mux.Router.ServeHTTP(recorder, request)
			test.check(t, recorder)
		})
	}
}
