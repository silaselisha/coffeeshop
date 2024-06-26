package internal

import (
	"context"
	"crypto"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/silaselisha/coffee-api/pkg/store"
	"github.com/silaselisha/coffee-api/types"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func LoadEnvs(path string) (config *types.Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName(".env")
	viper.SetConfigType("env")

	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	err = viper.Unmarshal(&config)
	if err != nil {
		return nil, err
	}

	return
}

func HandleFuncDecorator(handle func(ctx context.Context, w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handle(r.Context(), w, r)
	}
}

func Connect(ctx context.Context, envs *types.Config) (*mongo.Client, error) {
	rgx := regexp.MustCompile("<password>")
	URI := string(rgx.ReplaceAll([]byte(envs.DB_URI), []byte(envs.DB_PASSWORD)))

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(URI))
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, err
	}
	return client, nil
}

func ResponseHandler(w http.ResponseWriter, message interface{}, statusCode int) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	return json.NewEncoder(w).Encode(message)
}

func CreateNewProduct() store.Item {
	product := store.Item{
		Name:        "Caffe Latte",
		Price:       4.50,
		Description: "A cafe latte is a popular coffee drink that consists of espresso and steamed milk, topped with a thin layer of foam. It is perfect for those who enjoy a smooth and creamy coffee with a balanced flavor. At our coffee shop, we use high-quality beans and fresh milk to make our cafe lattes, and we can customize them with different syrups, spices, or whipped cream. ☕",
		Summary:     "A cafe latte is a coffee drink made with espresso and steamed milk, with a thin layer of foam on top. It has a smooth and creamy taste, and can be customized with different flavors. Our coffee shop offers high-quality and fresh cafe lattes for any occasion.🍵",
		Category:    "beverages",
		Ingridients: []string{"Espresso", "Milk", "Falvored syrup"},
	}

	return product
}

func CreateNewUser(email, name, phoneNumber, role string) store.User {
	user := store.User{
		UserName:    name,
		Email:       email,
		Password:    "abstarct&87",
		Role:        role,
		PhoneNumber: phoneNumber,
	}
	return user
}

func PasswordEncryption(password []byte) string {
	return fmt.Sprintf("%x", crypto.SHA256.New().Sum(password))
}

func ComparePasswordEncryption(password, comparePassword string) bool {
	hash := fmt.Sprintf("%x", crypto.SHA256.New().Sum([]byte(password)))
	return hash == comparePassword
}

func genObjectToken() (string, error) {
	buff := make([]byte, 16)
	_, err := rand.Read(buff)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes %w", err)
	}
	return fmt.Sprintf("%x", buff), nil
}

func ResetToken(expire int32) (timestamp int64) {
	duration := time.Minute * time.Duration(int(expire))
	expiryTime := time.Now().Add(duration)
	return expiryTime.Local().UnixMilli()
}

func ImageProcessor(ctx context.Context, file io.ReadCloser, opts *types.FileMetadata) (data []byte, fileName string, extension string, err error) {
	data, err = io.ReadAll(file)
	if err != nil {
		return
	}

	contetntType := strings.Split(http.DetectContentType(data), "/")
	if contetntType[0] != opts.ContetntType {
		err = fmt.Errorf("invalid file! required file is %+v", opts.ContetntType)
		return
	}

	extension = contetntType[1]
	objectName, err := genObjectToken()
	if err != nil {
		err = fmt.Errorf("failed to generate object token: %w", err)
		return
	}

	defer func() {
		if err := file.Close(); err != nil {
			log.Fatalf("Failed to close the file: %v\n", err)
			return
		}
	}()
	fileName = fmt.Sprintf("%s.%s", objectName, extension)
	return
}

func NewErrorResponse(status string, err string) *types.ErrorResParams {
	return &types.ErrorResParams{
		Status: status,
		Error:  err,
	}
}

func ReadReqBody[T types.UserReqParams | types.OrderParams | types.UserLoginParams | types.ForgotPasswordParams | types.PasswordResetParams](data io.ReadCloser, sanitizer *validator.Validate) (payload T, err error) {
	payloadBytes, err := io.ReadAll(data)
	if err != nil {
		if err == io.EOF {
			return payload, err
		}
		return payload, err
	}
	
	err = json.Unmarshal(payloadBytes, &payload)
	if err != nil {
		return payload, err
	}
	
	defer func(payload T) {
		if dErr := data.Close(); dErr != nil {
			err = dErr
			return
		}
	}(payload)
	
	err = sanitizer.Struct(payload)
	if err != nil {
		return payload, err
	}

	if err != nil {
		return payload, err
	}
	return payload, err
}

func ExtractProductsID(orders types.OrderParams) ([]primitive.ObjectID, []store.OrderItem, error) {
	var products []store.OrderItem
    var productsIds []primitive.ObjectID

	for _, order := range orders.Items {
		id, err := primitive.ObjectIDFromHex(order.Product)
		if err != nil {
			return nil, nil, err
		}
		item := store.OrderItem{
			Product: id,
			Quantity: order.Quantity,
		}

		productsIds = append(productsIds, id)
		products = append(products, item)
	}
	return productsIds, products, nil
}