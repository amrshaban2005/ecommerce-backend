package repository

import (
	"context"
	"errors"
	"fmt"

	"ecommerce.local/auth-service/internal/domain"
	"ecommerce.local/auth-service/internal/port"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresUserRepository struct{ pool *pgxpool.Pool }



func NewPostgresUserRepository(pool *pgxpool.Pool) port.UserRepository {
	return &PostgresUserRepository{pool: pool}
}

func (r *PostgresUserRepository) Create(ctx context.Context, user domain.User) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO users (id,full_name,email,password_hash,role) VALUES ($1,$2,$3,$4,$5)`, user.ID, user.FullName, user.Email, user.PasswordHash, user.Role)
	if err == nil {
		return nil
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23505" && databaseError.ConstraintName == "users_email_key" {
		return domain.ErrEmailAlreadyRegistered
	}
	return fmt.Errorf("create user: %w", err)
}

func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	var user domain.User
	err := r.pool.QueryRow(ctx, `SELECT id,full_name,email,password_hash,role FROM users WHERE email=$1`, email).Scan(&user.ID, &user.FullName, &user.Email, &user.PasswordHash, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("get user by email: %w", err)
	}
	return user, nil
}
