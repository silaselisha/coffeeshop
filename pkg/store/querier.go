package store

import (
	"context"
	"net/http"
)

type Querier interface {
	UsersQueries
	ProductsQueries
}

type UsersQueries interface{
	CreateUserHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	GetAllUsersHandlers(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	GetUserByIdHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	DeleteUserByIdHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	LoginUserHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	ForgotPasswordHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	ResetPasswordHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
}

type ProductsQueries interface {
	CreateProductHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	UpdateProductHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	DeleteProductByIdHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	GetAllProductsHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	GetProductByIdHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error
}