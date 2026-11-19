package port

import (
	"context"
	"ecommerce.local/product-service/internal/domain"
)

type CreateProductCommand struct {
	Name        string
	Description string
	CategoryID  string
	PriceUSD    string
	ImageURL    string
}

type ProductCatalog interface {
	Browse(context.Context) ([]domain.Product, error)
	Create(context.Context, CreateProductCommand) (domain.Product, error)
}

type ProductRepository interface {
	ListActive(context.Context) ([]domain.Product, error)
	Create(context.Context, domain.Product) (domain.Product, error)
}
