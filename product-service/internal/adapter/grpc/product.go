package grpcadapter

import (
	"context"
	"errors"
	"log/slog"

	productv1 "ecommerce.local/contracts/product/v1"
	"ecommerce.local/product-service/internal/domain"
	"ecommerce.local/product-service/internal/port"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ProductServer struct {
	productv1.UnimplementedProductServiceServer
	products port.ProductCatalog
}

func NewProductServer(products port.ProductCatalog) *ProductServer {
	return &ProductServer{products: products}
}

func (s *ProductServer) GetCheckoutProducts(ctx context.Context, req *productv1.GetCheckoutProductsRequest) (*productv1.GetCheckoutProductsResponse, error) {
	products, err := s.products.GetCheckoutProducts(ctx, req.GetProductIds())
	if err != nil {
		var validationError *domain.ValidationError
		switch {
		case errors.As(err, &validationError):
			return nil, status.Error(codes.InvalidArgument, validationError.Error())
		case errors.Is(err, context.Canceled):
			return nil, status.Error(codes.Canceled, "request canceled")
		case errors.Is(err, context.DeadlineExceeded):
			return nil, status.Error(codes.DeadlineExceeded, "product lookup deadline exceeded")
		default:
			slog.Error("checkout product lookup failed", "error", err, "service", "product-service")
			return nil, status.Error(codes.Internal, "product lookup failed")
		}
	}
	response := &productv1.GetCheckoutProductsResponse{Products: make([]*productv1.CheckoutProduct, 0, len(products))}
	for _, product := range products {
		response.Products = append(response.Products, &productv1.CheckoutProduct{
			ProductId: product.ID, Name: product.Name, Active: product.Active, PriceUsd: product.PriceUSD,
		})
	}
	return response, nil
}
