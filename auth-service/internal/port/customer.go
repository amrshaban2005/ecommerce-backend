package port

import (
	"context"
	"ecommerce.local/auth-service/internal/domain"
)

type RegisterCustomerCommand struct {
	FullName string
	Email    string
	Password string
}

type SignInCustomerCommand struct {
	Email    string
	Password string
}

type CustomerRegistration interface {
	Register(context.Context, RegisterCustomerCommand) (domain.User, error)
	SignIn(context.Context, SignInCustomerCommand) (domain.AccessToken, error)
	ValidateToken(context.Context, string) (domain.TokenVerification, error)
}

type UserRepository interface {
	Create(context.Context, domain.User) error
	GetByEmail(context.Context, string) (domain.User, error)
}
