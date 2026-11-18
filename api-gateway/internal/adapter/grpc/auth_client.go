// Package authclient calls the internal Auth token-verification service.
package grpcadapter

import (
	"context"
	"fmt"
	"time"

	authv1 "ecommerce.local/contracts/auth/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type Client struct {
	conn    *grpc.ClientConn
	client  authv1.AuthServiceClient
	timeout time.Duration
}

func NewAuthClient(address string, timeout time.Duration) (*Client, error) {
	if address == "" || timeout <= 0 {
		return nil, fmt.Errorf("Auth address and a positive verification timeout are required")
	}
	// The RPC is restricted to the private deployment network, like the reference client.
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("Auth grpc client: %w", err)
	}
	return &Client{conn: conn, client: authv1.NewAuthServiceClient(conn), timeout: timeout}, nil
}

func (c *Client) ValidateToken(ctx context.Context, token, correlationID string) (*authv1.ValidateTokenResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if correlationID != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-correlation-id", correlationID)
	}
	response, err := c.client.ValidateToken(ctx, &authv1.ValidateTokenRequest{Token: token})
	if err != nil {
		return nil, fmt.Errorf("validate token: %w", err)
	}
	if response == nil {
		return nil, fmt.Errorf("Auth returned no verification response")
	}
	return response, nil
}

func (c *Client) Close() error { return c.conn.Close() }
