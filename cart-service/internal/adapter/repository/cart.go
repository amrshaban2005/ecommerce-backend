package repository

import (
	"context"
	"errors"
	"fmt"

	"ecommerce.local/cart-service/internal/domain"
	"ecommerce.local/cart-service/internal/port"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresCartRepository struct{ pool *pgxpool.Pool }

func NewPostgresCartRepository(pool *pgxpool.Pool) port.CartRepository {
	return &PostgresCartRepository{pool: pool}
}

func (r *PostgresCartRepository) AddItem(ctx context.Context, cartID, customerID, productID string, quantity int32) (domain.CartItem, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.CartItem{}, fmt.Errorf("begin cart addition: %w", err)
	}
	defer tx.Rollback(ctx)

	// The unique customer key serializes concurrent creation and additions for one cart.
	err = tx.QueryRow(ctx, `
 INSERT INTO carts (id, customer_id) VALUES ($1, $2)
 ON CONFLICT (customer_id) DO UPDATE SET customer_id = EXCLUDED.customer_id
 RETURNING id`, cartID, customerID).Scan(&cartID)
	if err != nil {
		return domain.CartItem{}, fmt.Errorf("find or create cart: %w", err)
	}

	var item domain.CartItem
	err = tx.QueryRow(ctx, `
 INSERT INTO cart_items (cart_id, product_id, quantity) VALUES ($1, $2, $3)
 ON CONFLICT (cart_id, product_id) DO UPDATE
 SET quantity = cart_items.quantity + EXCLUDED.quantity
 WHERE cart_items.quantity <= 2147483647 - EXCLUDED.quantity
 RETURNING cart_id, product_id, quantity`, cartID, productID, quantity).
		Scan(&item.CartID, &item.ProductID, &item.Quantity)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CartItem{}, domain.ErrQuantityLimit
	}
	if err != nil {
		return domain.CartItem{}, fmt.Errorf("add cart item: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CartItem{}, fmt.Errorf("commit cart addition: %w", err)
	}
	return item, nil
}

func (r *PostgresCartRepository) ListByCustomer(ctx context.Context, customerID string) ([]domain.CartItem, error) {
	rows, err := r.pool.Query(ctx, `
 SELECT i.cart_id, i.product_id, i.quantity
 FROM cart_items i
 JOIN carts c ON c.id = i.cart_id
 WHERE c.customer_id = $1
 ORDER BY i.product_id`, customerID)
	if err != nil {
		return nil, fmt.Errorf("list customer cart items: %w", err)
	}
	defer rows.Close()
	items := make([]domain.CartItem, 0)
	for rows.Next() {
		var item domain.CartItem
		if err := rows.Scan(&item.CartID, &item.ProductID, &item.Quantity); err != nil {
			return nil, fmt.Errorf("read cart item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cart items: %w", err)
	}
	return items, nil
}

func (r *PostgresCartRepository) RemovePurchasedItems(ctx context.Context, cmd port.RemovePurchasedCommand, hash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	inserted, err := tx.Exec(ctx, `INSERT INTO processed_orders(order_id,customer_id,items_hash) VALUES($1,$2,$3)
 ON CONFLICT(order_id) DO NOTHING`, cmd.OrderID, cmd.CustomerID, hash)
	if err != nil {
		return err
	}
	if inserted.RowsAffected() == 0 {
		var customerID, itemsHash string
		if err := tx.QueryRow(ctx, `SELECT customer_id,items_hash FROM processed_orders WHERE order_id=$1`, cmd.OrderID).Scan(&customerID, &itemsHash); err != nil {
			return err
		}
		if customerID != cmd.CustomerID || itemsHash != hash {
			return &domain.ValidationError{Field: "order_id", Message: "purchased items do not match the processed order"}
		}
		return nil
	}
	var cartID string
	err = tx.QueryRow(ctx, `SELECT id FROM carts WHERE customer_id=$1 FOR UPDATE`, cmd.CustomerID).Scan(&cartID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil {
		// Cart additions lock this same customer cart row. Remove only purchased
		// quantities, preserving any excess or unrelated items added by the customer.
		for _, item := range cmd.Items {
			if _, err := tx.Exec(ctx, `DELETE FROM cart_items WHERE cart_id=$1 AND product_id=$2 AND quantity<=$3`, cartID, item.ProductID, item.Quantity); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE cart_items SET quantity=quantity-$3 WHERE cart_id=$1 AND product_id=$2`, cartID, item.ProductID, item.Quantity); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}
