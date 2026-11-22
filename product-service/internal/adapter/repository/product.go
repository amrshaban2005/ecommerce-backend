package repository

import (
	"context"
	"errors"
	"fmt"

	"ecommerce.local/product-service/internal/domain"
	"ecommerce.local/product-service/internal/port"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresProductRepository struct{ pool *pgxpool.Pool }

func NewPostgresProductRepository(pool *pgxpool.Pool) port.ProductRepository {
	return &PostgresProductRepository{pool: pool}
}

func (r *PostgresProductRepository) ListActive(ctx context.Context) ([]domain.Product, error) {
	// Stock must not be a filter: active out-of-stock products remain visible.
	rows, err := r.pool.Query(ctx, `
  SELECT p.id,p.name,p.description,p.category_id,c.name,p.price_usd::text,p.active,COALESCE(p.image_url,'')
  FROM products p
  JOIN categories c ON c.id=p.category_id
  WHERE p.active=TRUE
  ORDER BY p.id`)
	if err != nil {
		return nil, fmt.Errorf("list active products: %w", err)
	}
	defer rows.Close()
	products := make([]domain.Product, 0)
	for rows.Next() {
		var product domain.Product
		if err := rows.Scan(&product.ID, &product.Name, &product.Description, &product.CategoryID, &product.CategoryName, &product.PriceUSD, &product.Active, &product.ImageURL); err != nil {
			return nil, fmt.Errorf("read product: %w", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}
	return products, nil
}

func (r *PostgresProductRepository) Create(ctx context.Context, product domain.Product) (domain.Product, error) {
	var created domain.Product
	err := r.pool.QueryRow(ctx, `
  WITH created AS (
    INSERT INTO products (id,name,description,category_id,price_usd,active,image_url)
    VALUES ($1,$2,$3,$4,$5::numeric,$6,NULLIF($7,''))
    RETURNING id,name,description,category_id,price_usd,active,image_url
  )
  SELECT p.id,p.name,p.description,p.category_id,c.name,p.price_usd::text,p.active,COALESCE(p.image_url,'')
  FROM created p
  JOIN categories c ON c.id=p.category_id`,
		product.ID, product.Name, product.Description, product.CategoryID, product.PriceUSD, product.Active, product.ImageURL,
	).Scan(&created.ID, &created.Name, &created.Description, &created.CategoryID, &created.CategoryName, &created.PriceUSD, &created.Active, &created.ImageURL)
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23503" && databaseError.ConstraintName == "products_category_id_fkey" {
			return domain.Product{}, &domain.ValidationError{Field: "category_id", Message: "must reference an existing category"}
		}
		return domain.Product{}, fmt.Errorf("create product: %w", err)
	}
	return created, nil
}

// FindByIDs reads current prices and status directly from Product's database.
// Inactive products are included so Order can reject them explicitly.
func (r *PostgresProductRepository) FindByIDs(ctx context.Context, ids []string) ([]domain.Product, error) {
	rows, err := r.pool.Query(ctx, `
 SELECT id, name, active, price_usd::text FROM products
 WHERE id = ANY($1::uuid[]) ORDER BY id`, ids)
	if err != nil {
		return nil, fmt.Errorf("find checkout products: %w", err)
	}
	defer rows.Close()
	products := make([]domain.Product, 0)
	for rows.Next() {
		var product domain.Product
		if err := rows.Scan(&product.ID, &product.Name, &product.Active, &product.PriceUSD); err != nil {
			return nil, fmt.Errorf("read checkout product: %w", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checkout products: %w", err)
	}
	return products, nil
}
