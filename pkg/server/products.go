package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	"github.com/hibiken/asynq"
	"github.com/silaselisha/coffee-api/internal"
	"github.com/silaselisha/coffee-api/pkg/store"
	"github.com/silaselisha/coffee-api/types"
	"github.com/silaselisha/coffee-api/workers"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (s *Server) UpdateProductHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	session, err := s.Store.TxnStartSession(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if abortErr := session.AbortTransaction(ctx); abortErr != nil {
			err = abortErr
		}
	}()

	defer session.EndSession(ctx)
	response, err := session.WithTransaction(ctx, func(ctx mongo.SessionContext) (interface{}, error) {
		collection := s.Store.Collection(ctx, "coffeeshop", "products")
		vars := mux.Vars(r)
		id, err := primitive.ObjectIDFromHex(vars["id"])
		if err != nil {
			return nil, err
		}

		var item store.Item
		curr := collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}})
		err = curr.Decode(&item)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, err
			}
			return nil, err
		}

		var updates bson.M = bson.M{}
		reader, err := r.MultipartReader()
		if err != nil {
			return nil, err
		}

		for {
			curr, err := reader.NextPart()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}

			switch curr.FormName() {
			case "name":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}
				updates[curr.FormName()] = string(data)

			case "summary":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}
				updates[curr.FormName()] = string(data)

			case "description":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}
				updates[curr.FormName()] = string(data)

			case "price":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}

				price, err := strconv.ParseFloat(string(data), 64)
				if err != nil {
					return nil, err
				}

				updates[curr.FormName()] = price

			case "ingridients":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}
				updates[curr.FormName()] = strings.Split(string(data), ",")

			case "thumbnail":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}

				opts := []asynq.Option{
					asynq.MaxRetry(3),
					asynq.ProcessIn(2 * time.Second),
					asynq.Queue(workers.CriticalQueue),
				}

				if item.Thumbnail != "" {
					opts := []asynq.Option{
						asynq.MaxRetry(3),
						asynq.ProcessIn(3 * time.Minute),
						asynq.Queue(workers.CriticalQueue),
					}

					thumbnails := []string{item.Thumbnail}
					err := s.taskDistributor.S3ObjectDeleteTask(ctx, thumbnails, opts...)
					if err != nil {
						return nil, err
					}
				}

				imageFile := io.NopCloser(bytes.NewReader(data))
				image, fileName, extension, err := internal.ImageProcessor(ctx, imageFile, &types.FileMetadata{ContetntType: "image"})
				if err != nil {
					return nil, err
				}

				objectKey := fmt.Sprintf("images/products/thumbnails/%s", fileName)
				err = s.taskDistributor.S3ObjectUploadTask(ctx, &types.PayloadUploadImage{
					Image:     image,
					Extension: extension,
					ObjectKey: objectKey,
				}, opts...)
				if err != nil {
					return nil, err
				}
				updates[curr.FormName()] = objectKey
			}

		}

		var updatedDocument store.Item
		filter := bson.D{{Key: "_id", Value: id}}
		updates["updated_at"] = time.Now()
		update := bson.M{"$set": updates}

		newDocs := options.After
		err = collection.FindOneAndUpdate(ctx, filter, update, &options.FindOneAndUpdateOptions{
			ReturnDocument: &newDocs,
		}).Decode(&updatedDocument)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, err
			}
			return nil, err
		}

		product := types.ItemResParams{
			Id:          updatedDocument.Id.Hex(),
			Images:      updatedDocument.Images,
			Name:        updatedDocument.Name,
			Price:       updatedDocument.Price,
			Discount:    updatedDocument.Discount,
			Summary:     updatedDocument.Summary,
			Category:    updatedDocument.Category,
			Thumbnail:   updatedDocument.Thumbnail,
			Description: updatedDocument.Description,
			Ingridients: updatedDocument.Ingridients,
			Ratings:     updatedDocument.Ratings,
			CreatedAt:   updatedDocument.CreatedAt,
			UpdatedAt:   updatedDocument.UpdatedAt,
		}

		err = session.CommitTransaction(ctx)
		if err != nil {
			return nil, err
		}

		return product, nil
	}, &options.TransactionOptions{})

	if err != nil {
		switch {
		case errors.As(err, &mongo.WriteException{}):
			exception, _ := err.(mongo.WriteException)
			if exception.WriteErrors[0].Code == 11000 {
				return internal.ResponseHandler(w, internal.NewErrorResponse("failed", fmt.Errorf("document already exists %w", err).Error()), http.StatusBadRequest)
			}

		case errors.Is(err, mongo.ErrNoDocuments):
			if err == mongo.ErrNoDocuments {
				return internal.ResponseHandler(w, internal.NewErrorResponse("failed", fmt.Errorf("document not found %w", err).Error()), http.StatusNotFound)
			}

		case errors.Is(err, &json.SyntaxError{}):
			return internal.ResponseHandler(w, internal.NewErrorResponse("failed", fmt.Errorf("ivalid data input for operation %w", err).Error()), http.StatusBadRequest)

		case err.(validator.ValidationErrors) != nil:
			return internal.ResponseHandler(w, internal.NewErrorResponse("failed", fmt.Errorf("ivalid data input for operation %w", err).Error()), http.StatusBadRequest)

		default:
			return internal.ResponseHandler(w, internal.NewErrorResponse("failed", err.Error()), http.StatusInternalServerError)
		}
	}

	result := struct {
		Status string              `json:"status"`
		Data   types.ItemResParams `json:"data"`
	}{
		Status: "success",
		Data:   response.(types.ItemResParams),
	}
	return internal.ResponseHandler(w, result, http.StatusOK)
}

func (s *Server) GetAllProductsHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	collection := s.Store.Collection(ctx, "coffeeshop", "products")

	var result types.ItemResponseListParams
	type resp struct {
		Status  string                       `json:"status"`
		Results int32                        `json:"results"`
		Data    types.ItemResponseListParams `json:"data"`
	}

	productRes := resp{Status: "success", Results: 0, Data: result}
	cur, err := collection.Find(ctx, bson.D{})
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return internal.ResponseHandler(w, productRes, http.StatusOK)
		}

		return internal.ResponseHandler(w, internal.NewErrorResponse("failed", err.Error()), http.StatusInternalServerError)
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		item := new(store.Item)
		err := cur.Decode(&item)
		if err != nil {
			if err == io.EOF {
				break
			}
			return internal.ResponseHandler(w, internal.NewErrorResponse("failed", err.Error()), http.StatusInternalServerError)
		}

		product := types.ItemResParams{
			Id:          item.Id.Hex(),
			Images:      item.Images,
			Name:        item.Name,
			Author:      item.Author,
			Price:       item.Price,
			Discount:    item.Discount,
			Summary:     item.Summary,
			Category:    item.Category,
			Thumbnail:   item.Thumbnail,
			Description: item.Description,
			Ingridients: item.Ingridients,
			Ratings:     item.Ratings,
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
		}
		result = append(result, product)
	}

	productRes = resp{Status: "success", Results: int32(len(result)), Data: result}
	return internal.ResponseHandler(w, productRes, http.StatusOK)
}

func (s *Server) GetProductByIdHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	collection := s.Store.Collection(ctx, "coffeeshop", "products")

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		return internal.ResponseHandler(w, internal.NewErrorResponse("failed", err.Error()), http.StatusBadRequest)
	}

	filter := bson.D{{Key: "_id", Value: id}, {Key: "category", Value: vars["category"]}}
	result := collection.FindOne(ctx, filter)

	var item store.Item
	err = result.Decode(&item)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return internal.ResponseHandler(w, internal.NewErrorResponse("failed", fmt.Errorf("document not found %w", err).Error()), http.StatusNotFound)
		}
		return internal.ResponseHandler(w, internal.NewErrorResponse("failed", err.Error()), http.StatusInternalServerError)
	}

	product := types.ItemResParams{
		Id:          item.Id.Hex(),
		Images:      item.Images,
		Name:        item.Name,
		Author:      item.Author,
		Price:       item.Price,
		Discount:    item.Discount,
		Summary:     item.Summary,
		Category:    item.Category,
		Thumbnail:   item.Thumbnail,
		Description: item.Description,
		Ingridients: item.Ingridients,
		Ratings:     item.Ratings,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}

	res := struct {
		Status string              `json:"status"`
		Data   types.ItemResParams `json:"data"`
	}{
		Status: "success",
		Data:   product,
	}
	return internal.ResponseHandler(w, res, http.StatusOK)
}

func (s *Server) DeleteProductByIdHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	session, err := s.Store.TxnStartSession(ctx)
	if err != nil {
		return err
	}

	defer session.EndSession(ctx)
	defer func() {
		if abortErr := session.AbortTransaction(ctx); abortErr != nil {
			err = abortErr
		}
	}()

	_, err = session.WithTransaction(ctx, func(ctx mongo.SessionContext) (interface{}, error) {
		collection := s.Store.Collection(ctx, "coffeeshop", "products")
		params := mux.Vars(r)
		id, err := primitive.ObjectIDFromHex(params["id"])
		if err != nil {
			return nil, err
		}

		var product store.Item
		curr := collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}})
		err = curr.Decode(&product)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, err
			}
			return nil, err
		}

		var images []string
		images = append(images, product.Images...)
		images = append(images, product.Thumbnail)

		opts := []asynq.Option{
			asynq.ProcessIn(1 * time.Minute),
			asynq.MaxRetry(3),
			asynq.Queue(workers.CriticalQueue),
		}

		err = s.taskDistributor.S3ObjectDeleteTask(ctx, images, opts...)
		if err != nil {
			return nil, err
		}

		filter := bson.D{{Key: "_id", Value: id}}
		var deletedDocument bson.M
		err = collection.FindOneAndDelete(ctx, filter).Decode(&deletedDocument)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, err
			}
			return err, nil
		}

		err = session.CommitTransaction(ctx)
		if err != nil {
			return nil, err
		}

		return nil, nil
	}, &options.TransactionOptions{})

	if err != nil {
		switch {
		case errors.As(err, &mongo.WriteException{}):
			exception, _ := err.(mongo.WriteException)
			if exception.WriteErrors[0].Code == 11000 {
				return internal.ResponseHandler(w, internal.NewErrorResponse("failed", fmt.Errorf("document already exists %w", err).Error()), http.StatusBadRequest)
			}

		case errors.Is(err, mongo.ErrNoDocuments):
			return internal.ResponseHandler(w, internal.NewErrorResponse("failed", fmt.Errorf("document not found %w", err).Error()), http.StatusNotFound)

		default:
			return internal.ResponseHandler(w, internal.NewErrorResponse("failed", err.Error()), http.StatusInternalServerError)
		}
	}

	return internal.ResponseHandler(w, "", http.StatusNoContent)
}

func (s *Server) CreateProductHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	session, err := s.Store.TxnStartSession(ctx)
	if err != nil {
		return session.AbortTransaction(ctx)
	}

	defer func() {
		if abortError := session.AbortTransaction(ctx); err != nil {
			err = abortError
		}
	}()

	defer session.EndSession(ctx)
	resposne, err := session.WithTransaction(ctx, func(ctx mongo.SessionContext) (interface{}, error) {
		collection := s.Store.Collection(ctx, "coffeeshop", "products")
		_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: "name", Value: 1}},
			Options: options.Index().SetUnique(true),
		})

		if err != nil {
			return nil, err
		}

		var item store.Item
		var images []*types.PayloadUploadImage
		reader, err := r.MultipartReader()
		if err != nil {
			return nil, err
		}

		userInfo := ctx.Value(types.AuthUserInfoKey{}).(*types.UserInfo)
		item.Author = userInfo.Id

		for {
			curr, err := reader.NextPart()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}

			switch curr.FormName() {
			case "price":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}
				price, err := strconv.ParseFloat(string(data), 64)
				if err != nil {
					return nil, err
				}
				item.Price = price

			case "discount":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}
				discount, err := strconv.ParseUint(string(data), 10, 32)
				if err != nil {
					return nil, err
				}
				item.Discount = uint32(discount)
			case "ingridients":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}
				ingridients := strings.Split(string(data), ",")
				item.Ingridients = ingridients

			case "name":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}
				item.Name = string(data)

			case "summary":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}
				item.Summary = string(data)

			case "description":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}
				item.Description = string(data)

			case "category":
				data, err := io.ReadAll(curr)
				if err != nil {
					return nil, err
				}
				item.Category = string(data)

			case "thumbnail":
				data, fileName, extension, err := internal.ImageProcessor(ctx, curr, &types.FileMetadata{ContetntType: "image"})
				if err != nil {
					return nil, err
				}

				objectKey := fmt.Sprintf("images/products/thumbnails/%s", fileName)
				item.Thumbnail = objectKey
				opts := []asynq.Option{
					asynq.MaxRetry(3),
					asynq.ProcessIn(2 * time.Second),
					asynq.Queue(workers.CriticalQueue),
				}

				err = s.taskDistributor.S3ObjectUploadTask(ctx, &types.PayloadUploadImage{
					ObjectKey: objectKey,
					Extension: extension,
					Image:     data,
				}, opts...)

				if err != nil {
					return nil, err
				}

			case "images":
				data, fileName, extension, err := internal.ImageProcessor(ctx, curr, &types.FileMetadata{ContetntType: "image"})
				if err != nil {
					return nil, err
				}
				objectKey := fmt.Sprintf("images/products/beverages/%s", fileName)
				images = append(images, &types.PayloadUploadImage{
					Image:     data,
					Extension: extension,
					ObjectKey: objectKey,
				})
				item.Images = append(item.Images, objectKey)
			}
		}

		opts := []asynq.Option{
			asynq.MaxRetry(3),
			asynq.ProcessIn(2 * time.Second),
			asynq.Queue(workers.CriticalQueue),
		}
		err = s.taskDistributor.MultipleS3ObjectUploadTask(ctx, images, opts...)
		if err != nil {
			return nil, err
		}

		item.Id = primitive.NewObjectID()
		item.CreatedAt = time.Now()
		item.UpdatedAt = time.Now()
		if err := s.vd.Struct(&item); err != nil {
			return nil, err
		}

		_, err = collection.InsertOne(ctx, item)
		if err != nil {
			return nil, err
		}

		product := types.ItemResParams{
			Id:          item.Id.Hex(),
			Images:      item.Images,
			Name:        item.Name,
			Author:      item.Author,
			Price:       item.Price,
			Discount:    item.Discount,
			Ingridients: item.Ingridients,
			Thumbnail:   item.Thumbnail,
			Summary:     item.Summary,
			Category:    item.Category,
			Description: item.Description,
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
		}

		err = session.CommitTransaction(ctx)
		if err != nil {
			return nil, err
		}

		return product, nil
	}, &options.TransactionOptions{})

	if err != nil {
		switch {
		case errors.As(err, &mongo.WriteException{}):
			exceptionError, _ := err.(mongo.WriteException)
			if exceptionError.WriteErrors[0].Code == 11000 {
				return internal.ResponseHandler(w, internal.NewErrorResponse("failed", fmt.Errorf("document already exists %w", err).Error()), http.StatusBadRequest)
			}

		case errors.Is(err, &json.SyntaxError{}):
			return internal.ResponseHandler(w, internal.NewErrorResponse("failed", fmt.Errorf("invalid data input for operation %w", err).Error()), http.StatusBadRequest)

		case err.(validator.ValidationErrors) != nil:
			return internal.ResponseHandler(w, internal.NewErrorResponse("failed", fmt.Errorf("ivalid data input for operation %w", err).Error()), http.StatusBadRequest)

		default:
			return internal.ResponseHandler(w, internal.NewErrorResponse("failed", err.Error()), http.StatusInternalServerError)
		}
	}

	product := resposne.(types.ItemResParams)
	result := struct {
		Status string              `json:"status"`
		Data   types.ItemResParams `json:"data"`
	}{
		Status: "success",
		Data:   product,
	}

	return internal.ResponseHandler(w, result, http.StatusCreated)
}

func (s *Server) BatchGetAllProductsByIds(ctx context.Context, data []primitive.ObjectID) (products map[primitive.ObjectID]store.Item, err error) {
	prodColl := s.Store.Collection(ctx, "coffeeshop", "products")

	cur, err := prodColl.Find(ctx, bson.M{"_id": bson.M{"$in": data}})
	if err != nil {
		return nil, err
	}

	defer func() {
		if dErr := cur.Close(ctx); dErr != nil {
			err = dErr
		}
	}()

	products = make(map[primitive.ObjectID]store.Item)
	for cur.Next(ctx) {
		var item store.Item
		err := cur.Decode(&item)
		if err != nil {
			return nil, err
		}

		products[item.Id] = item
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}
	return
}


