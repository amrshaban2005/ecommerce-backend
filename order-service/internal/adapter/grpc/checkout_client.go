package grpcadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	cartv1 "ecommerce.local/contracts/cart/v1"
	inventoryv1 "ecommerce.local/contracts/inventory/v1"
	productv1 "ecommerce.local/contracts/product/v1"
	"ecommerce.local/order-service/internal/port"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// CheckoutClient implements outbound ports without exposing protobufs to the use case.
// Connections are restricted to the private service network, following Auth's client.
type CheckoutClient struct {
	connections []*grpc.ClientConn
	cart        cartv1.CartServiceClient
	product     productv1.ProductServiceClient
	inventory   inventoryv1.InventoryServiceClient
	timeout     time.Duration
}

var (
	_ port.CartReader            = (*CheckoutClient)(nil)
	_ port.ProductReader         = (*CheckoutClient)(nil)
	_ port.InventoryReservations = (*CheckoutClient)(nil)
)

func NewCheckoutClient(cartAddress, productAddress, inventoryAddress string, timeout time.Duration) (*CheckoutClient, error) {
	if strings.TrimSpace(cartAddress) == "" || strings.TrimSpace(productAddress) == "" || strings.TrimSpace(inventoryAddress) == "" || timeout <= 0 {
		return nil, fmt.Errorf("Cart, Product and Inventory addresses and a positive RPC timeout are required")
	}
	client := &CheckoutClient{timeout: timeout}
	for _, address := range []string{cartAddress, productAddress, inventoryAddress} {
		conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("checkout grpc client: %w", err)
		}
		client.connections = append(client.connections, conn)
	}
	client.cart = cartv1.NewCartServiceClient(client.connections[0])
	client.product = productv1.NewProductServiceClient(client.connections[1])
	client.inventory = inventoryv1.NewInventoryServiceClient(client.connections[2])
	return client, nil
}

func (c *CheckoutClient) Close() error {
	var errs []error
	for _, conn := range c.connections {
		if err := conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *CheckoutClient) GetCustomerCart(ctx context.Context, customerID string) ([]port.CartItem, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.cart.GetCustomerCart(ctx, &cartv1.GetCustomerCartRequest{CustomerId: customerID})
	if err != nil {
		return nil, &port.DependencyError{Service: "Cart", Err: err}
	}
	if response == nil {
		return nil, &port.DependencyError{Service: "Cart", Err: errors.New("no response")}
	}
	items := make([]port.CartItem, 0, len(response.GetItems()))
	for _, item := range response.GetItems() {
		items = append(items, port.CartItem{ProductID: item.GetProductId(), Quantity: item.GetQuantity()})
	}
	return items, nil
}

func (c *CheckoutClient) GetCheckoutProducts(ctx context.Context, ids []string) ([]port.CheckoutProduct, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.product.GetCheckoutProducts(ctx, &productv1.GetCheckoutProductsRequest{ProductIds: ids})
	if err != nil {
		return nil, &port.DependencyError{Service: "Product", Err: err}
	}
	if response == nil {
		return nil, &port.DependencyError{Service: "Product", Err: errors.New("no response")}
	}
	products := make([]port.CheckoutProduct, 0, len(response.GetProducts()))
	for _, product := range response.GetProducts() {
		products = append(products, port.CheckoutProduct{
			ProductID: product.GetProductId(), Name: product.GetName(), Active: product.GetActive(), PriceUSD: product.GetPriceUsd(),
		})
	}
	return products, nil
}

func (c *CheckoutClient) ReserveStock(ctx context.Context, orderID string, items []port.CartItem) (port.Reservation, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	request := &inventoryv1.ReserveStockRequest{OrderId: orderID, Items: make([]*inventoryv1.ReservationItem, 0, len(items))}
	for _, item := range items {
		request.Items = append(request.Items, &inventoryv1.ReservationItem{ProductId: item.ProductID, Quantity: item.Quantity})
	}
	response, err := c.inventory.ReserveStock(ctx, request)
	if err != nil {
		return port.Reservation{}, inventoryError(err)
	}
	return decodeReservation(response)
}

func (c *CheckoutClient) GetReservation(ctx context.Context, orderID string) (port.Reservation, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.inventory.GetReservation(ctx, &inventoryv1.GetReservationRequest{OrderId: orderID})
	if err != nil {
		return port.Reservation{}, inventoryError(err)
	}
	return decodeReservation(response)
}

func (c *CheckoutClient) ReleaseReservation(ctx context.Context, orderID string) (port.Reservation, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.inventory.ReleaseReservation(ctx, &inventoryv1.ReleaseReservationRequest{OrderId: orderID})
	if err != nil {
		return port.Reservation{}, inventoryError(err)
	}
	return decodeReservation(response)
}

func decodeReservation(response *inventoryv1.Reservation) (port.Reservation, error) {
	if response == nil || response.GetReservationId() == "" || response.GetOrderId() == "" || response.GetExpiresAt() == nil {
		return port.Reservation{}, &port.DependencyError{Service: "Inventory", Err: errors.New("invalid reservation")}
	}
	if err := response.GetExpiresAt().CheckValid(); err != nil {
		return port.Reservation{}, &port.DependencyError{Service: "Inventory", Err: fmt.Errorf("invalid expiry: %w", err)}
	}
	var state string
	switch response.GetStatus() {
	case inventoryv1.ReservationStatus_RESERVATION_STATUS_ACTIVE:
		state = "ACTIVE"
	case inventoryv1.ReservationStatus_RESERVATION_STATUS_FINALIZED:
		state = "FINALIZED"
	case inventoryv1.ReservationStatus_RESERVATION_STATUS_RELEASED:
		state = "RELEASED"
	default:
		return port.Reservation{}, &port.DependencyError{Service: "Inventory", Err: errors.New("unknown reservation status")}
	}
	reservation := port.Reservation{
		ID: response.GetReservationId(), OrderID: response.GetOrderId(), Status: state,
		ExpiresAt: response.GetExpiresAt().AsTime(), Items: make([]port.CartItem, 0, len(response.GetItems())),
	}
	for _, item := range response.GetItems() {
		reservation.Items = append(reservation.Items, port.CartItem{ProductID: item.GetProductId(), Quantity: item.GetQuantity()})
	}
	return reservation, nil
}

func inventoryError(err error) error {
	switch status.Code(err) {
	case codes.ResourceExhausted:
		return port.ErrInsufficientStock
	case codes.NotFound:
		return port.ErrReservationNotFound
	case codes.AlreadyExists:
		return port.ErrReservationConflict
	case codes.FailedPrecondition:
		return port.ErrReservationUnavailable
	default:
		return &port.DependencyError{Service: "Inventory", Err: err}
	}
}

func (c *CheckoutClient) FinalizeReservation(ctx context.Context, orderID string) (port.Reservation, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.inventory.FinalizeReservation(ctx, &inventoryv1.FinalizeReservationRequest{OrderId: orderID})
	if err != nil {
		return port.Reservation{}, inventoryError(err)
	}
	return decodeReservation(response)
}
func (c *CheckoutClient) RemovePurchasedItems(ctx context.Context, orderID, customerID string, items []port.CartItem) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	request := &cartv1.RemovePurchasedItemsRequest{OrderId: orderID, CustomerId: customerID, Items: make([]*cartv1.CartItem, 0, len(items))}
	for _, item := range items {
		request.Items = append(request.Items, &cartv1.CartItem{ProductId: item.ProductID, Quantity: item.Quantity})
	}
	_, err := c.cart.RemovePurchasedItems(ctx, request)
	if err != nil {
		return &port.DependencyError{Service: "Cart", Err: err}
	}
	return nil
}
