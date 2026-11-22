package application

import (
	"context"
	"fmt"

	"ecommerce.local/product-service/internal/domain"
	"ecommerce.local/product-service/internal/port"
	"github.com/google/uuid"
)

type ProductService struct{ products port.ProductRepository }

func NewProductService(products port.ProductRepository) port.ProductCatalog {
	return &ProductService{products: products}
}

func (s *ProductService) Browse(ctx context.Context) ([]domain.Product, error) {
	return s.products.ListActive(ctx)
}

func (s *ProductService) Create(ctx context.Context, cmd port.CreateProductCommand) (domain.Product, error) {
	product, err := domain.NormalizeProduct(cmd.Name, cmd.Description, cmd.CategoryID, cmd.PriceUSD, cmd.ImageURL)
	if err != nil {
		return domain.Product{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return domain.Product{}, fmt.Errorf("create product ID: %w", err)
	}
	product.ID = id.String()
	product.Active = true
	return s.products.Create(ctx, product)
}

func (s *ProductService) GetCheckoutProducts(ctx context.Context, ids []string) ([]domain.Product, error) {
	ids, err := domain.NormalizeProductIDs(ids)
	if err != nil {
		return nil, err
	}
	return s.products.FindByIDs(ctx, ids)
}
