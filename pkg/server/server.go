package server

import (
	"context"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	"github.com/silaselisha/coffee-api/internal"
	"github.com/silaselisha/coffee-api/internal/aws"
	"github.com/silaselisha/coffee-api/pkg/client"
	"github.com/silaselisha/coffee-api/pkg/store"
	"github.com/silaselisha/coffee-api/types"
	"github.com/silaselisha/coffee-api/pkg/token"
	"github.com/silaselisha/coffee-api/workers"
	"go.mongodb.org/mongo-driver/mongo"
)

type Server struct {
	Router             *mux.Router
	Store              store.Mongo
	coffeeShopS3Bucket aws.CoffeeShopBucket
	vd                 *validator.Validate
	envs               *types.Config
	Token              token.Token
	taskDistributor    workers.TaskDistributor
}

func NewServer(ctx context.Context, envs *types.Config, mongoClient *mongo.Client, distributor workers.TaskDistributor, templQueries client.Querier, fileServer func() http.Handler) store.Querier {
	server := &Server{}

	newServerHelper(ctx, envs, mongoClient, server, distributor)

	router := mux.NewRouter()

	render(router, templQueries, fileServer) // serve static files

	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	productRoutes(apiRouter, server)
	userRoutes(apiRouter, server)
	orderRoutes(apiRouter, server)

	server.Router = router
	return server
}

func newServerHelper(ctx context.Context, envs *types.Config, mongoClient *mongo.Client, server *Server, distributor workers.TaskDistributor) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Panic(err)
	}

	coffeShopS3Bucket := aws.NewS3Client(cfg, func(o *s3.Options) {
		o.Region = "us-east-1"
	})

	tkn := token.NewToken(envs.SECRET_ACCESS_KEY)
	store := store.NewMongoClient(mongoClient)
	server.coffeeShopS3Bucket = coffeShopS3Bucket
	server.Store = store
	server.envs = envs
	server.Token = tkn
	server.taskDistributor = distributor

	validate := validator.New(validator.WithRequiredStructEnabled())
	server.vd = validate
}

func render(router *mux.Router, templQueries client.Querier, fileServer func() http.Handler) {
	router.PathPrefix("/public/").Handler(fileServer())
	router.HandleFunc("/", internal.HandleFuncDecorator(templQueries.RenderHomePageHandler))
	router.HandleFunc("/about", internal.HandleFuncDecorator(templQueries.RenderAboutPageHandler))
}
