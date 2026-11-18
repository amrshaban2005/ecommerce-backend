package grpcadapter

import (
	"context"
	"errors"

	"ecommerce.local/auth-service/internal/port"
	authv1 "ecommerce.local/contracts/auth/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AuthServer struct {
	authv1.UnimplementedAuthServiceServer
	customers port.CustomerRegistration
}

func NewAuthServer(customers port.CustomerRegistration) *AuthServer {
	return &AuthServer{customers: customers}
}

func (s *AuthServer) ValidateToken(ctx context.Context, req *authv1.ValidateTokenRequest) (*authv1.ValidateTokenResponse, error) {
	identity, err := s.customers.ValidateToken(ctx, req.GetToken())
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return nil, status.Error(codes.Canceled, "request canceled")
		case errors.Is(err, context.DeadlineExceeded):
			return nil, status.Error(codes.DeadlineExceeded, "verification deadline exceeded")
		default:
			return nil, status.Error(codes.Internal, "token verification unavailable")
		}
	}
	if !identity.Valid {
		return &authv1.ValidateTokenResponse{Valid: false}, nil
	}
	return &authv1.ValidateTokenResponse{Valid: true, UserId: identity.UserID, Role: identity.Role}, nil
}
