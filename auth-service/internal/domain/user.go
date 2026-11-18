package domain

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"

	"golang.org/x/crypto/argon2"
)

const CustomerRole = "Customer"

var (
	ErrEmailAlreadyRegistered = errors.New("email already registered")
	ErrUserNotFound           = errors.New("user not found")
	ErrInvalidCredentials     = errors.New("invalid email or password")
	ErrInvalidPasswordHash    = errors.New("invalid stored password hash")
)

type User struct {
	ID           string
	FullName     string
	Email        string
	PasswordHash string
	Role         string
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// NormalizeRegistration leaves the password untouched.
func NormalizeRegistration(fullName, email, password string) (string, string, error) {
	fullName = strings.TrimSpace(fullName)
	email = strings.ToLower(strings.TrimSpace(email))
	if fullName == "" {
		return "", "", &ValidationError{Field: "full_name", Message: "is required"}
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || parsed.Name != "" {
		return "", "", &ValidationError{Field: "email", Message: "must be a valid email address"}
	}
	if utf8.RuneCountInString(password) < 8 {
		return "", "", &ValidationError{Field: "password", Message: "must contain at least 8 characters"}
	}
	return fullName, email, nil
}

func Hash(ctx context.Context, password string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	const memory uint32 = 19 * 1024
	const iterations uint32 = 2
	const parallelism uint8 = 1
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, 32)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, memory, iterations, parallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

// NormalizeSignIn follows registration's email normalization without changing the password.
func NormalizeSignIn(email, password string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || parsed.Name != "" {
		return "", &ValidationError{Field: "email", Message: "must be a valid email address"}
	}
	if password == "" {
		return "", &ValidationError{Field: "password", Message: "is required"}
	}
	return email, nil
}

// VerifyPassword verifies the encoded Argon2id format produced by Hash.
// Only the service's existing parameters are accepted, preventing corrupted
// stored parameters from requesting unbounded hashing resources.
func VerifyPassword(ctx context.Context, password, encodedHash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=19456,t=2,p=1" {
		return false, ErrInvalidPasswordHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false, ErrInvalidPasswordHash
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) != 32 {
		return false, ErrInvalidPasswordHash
	}
	actual := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

const (
	TokenIssuer   = "ecommerce-auth"
	TokenAudience = "ecommerce-api"
	TokenLifetime = 30 * time.Minute
)

type AccessToken struct {
	AccessToken string
	TokenType   string
	ExpiresIn   int64
}

type TokenClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateAccessToken signs only the verified identity; passwords are never claims.
func GenerateAccessToken(ctx context.Context, user User, secret string) (AccessToken, error) {
	if err := ctx.Err(); err != nil {
		return AccessToken{}, err
	}
	if len(secret) < 32 {
		return AccessToken{}, errors.New("JWT_SECRET must contain at least 32 bytes")
	}
	if user.ID == "" || (user.Role != CustomerRole && user.Role != "Administrator") {
		return AccessToken{}, errors.New("cannot issue token for invalid user identity")
	}
	now := time.Now().UTC().Truncate(time.Second)
	claims := TokenClaims{
		Role: user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			Issuer:    TokenIssuer,
			Audience:  jwt.ClaimStrings{TokenAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(TokenLifetime)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return AccessToken{}, fmt.Errorf("sign access token: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return AccessToken{}, err
	}
	return AccessToken{AccessToken: signed, TokenType: "Bearer", ExpiresIn: int64(TokenLifetime / time.Second)}, nil
}

type TokenVerification struct {
	Valid  bool
	UserID string
	Role   string
}

// VerifyAccessToken returns no identity for invalid tokens. Operational errors
// remain errors so the gateway can distinguish verification failure from denial.
func VerifyAccessToken(ctx context.Context, rawToken, secret string) (TokenVerification, error) {
	if err := ctx.Err(); err != nil {
		return TokenVerification{}, err
	}
	if len(secret) < 32 {
		return TokenVerification{}, errors.New("JWT_SECRET must contain at least 32 bytes")
	}
	if rawToken == "" {
		return TokenVerification{}, nil
	}
	claims := &TokenClaims{}
	token, err := jwt.ParseWithClaims(rawToken, claims, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(TokenIssuer),
		jwt.WithAudience(TokenAudience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return TokenVerification{}, ctxErr
	}
	if err != nil || token == nil || !token.Valid {
		return TokenVerification{}, nil
	}
	if strings.TrimSpace(claims.Subject) == "" || claims.IssuedAt == nil || claims.ExpiresAt == nil ||
		(claims.Role != CustomerRole && claims.Role != "Administrator") {
		return TokenVerification{}, nil
	}
	lifetime := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	if lifetime <= 0 || lifetime > TokenLifetime {
		return TokenVerification{}, nil
	}
	return TokenVerification{Valid: true, UserID: claims.Subject, Role: claims.Role}, nil
}
