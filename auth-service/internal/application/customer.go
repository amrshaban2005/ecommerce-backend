package application

import (
	"context"
	"errors"
	"fmt"

	"ecommerce.local/auth-service/internal/domain"
	"ecommerce.local/auth-service/internal/port"
	"github.com/google/uuid"
)

type RegistrationService struct {
	users     port.UserRepository
	jwtSecret string
}

func NewRegistrationService(users port.UserRepository, jwtSecret string) port.CustomerRegistration {
	return &RegistrationService{users: users, jwtSecret: jwtSecret}
}

func (s *RegistrationService) Register(ctx context.Context, cmd port.RegisterCustomerCommand) (domain.User, error) {
	fullName, email, err := domain.NormalizeRegistration(cmd.FullName, cmd.Email, cmd.Password)
	if err != nil {
		return domain.User{}, err
	}
	passwordHash, err := domain.Hash(ctx, cmd.Password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return domain.User{}, fmt.Errorf("create user ID: %w", err)
	}
	user := domain.User{ID: id.String(), FullName: fullName, Email: email, PasswordHash: passwordHash, Role: domain.CustomerRole}
	if err := s.users.Create(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *RegistrationService) SignIn(ctx context.Context, cmd port.SignInCustomerCommand) (domain.AccessToken, error) {
	email, err := domain.NormalizeSignIn(cmd.Email, cmd.Password)
	if err != nil {
		return domain.AccessToken{}, err
	}
	user, err := s.users.GetByEmail(ctx, email)
	if errors.Is(err, domain.ErrUserNotFound) {
		// Perform the same Argon2id work for unknown users to reduce timing differences.
		if _, hashErr := domain.Hash(ctx, cmd.Password); hashErr != nil {
			return domain.AccessToken{}, fmt.Errorf("verify credentials: %w", hashErr)
		}
		return domain.AccessToken{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return domain.AccessToken{}, err
	}
	valid, err := domain.VerifyPassword(ctx, cmd.Password, user.PasswordHash)
	if err != nil {
		return domain.AccessToken{}, fmt.Errorf("verify password: %w", err)
	}
	if !valid {
		return domain.AccessToken{}, domain.ErrInvalidCredentials
	}
	return domain.GenerateAccessToken(ctx, user, s.jwtSecret)
}

func (s *RegistrationService) ValidateToken(ctx context.Context, token string) (domain.TokenVerification, error) {
	return domain.VerifyAccessToken(ctx, token, s.jwtSecret)
}
