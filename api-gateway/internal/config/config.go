package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPPort               int
	RedisHost              string
	RedisPort              int
	RateLimitPerIP         int
	MaxRequestBodyBytes    int64
	UpstreamTimeoutSeconds int
	AuthHTTPAddress        string
	ProductHTTPAddress     string
	CartHTTPAddress        string
	InventoryHTTPAddress   string
	OrderHTTPAddress       string
	PaymentHTTPAddress     string
}

func Load() (*Config, error) {
	var errs []error

	httpPort, err := getInt("HTTP_PORT")
	if err != nil {
		errs = append(errs, err)
	} else if err := checkPort(httpPort); err != nil {
		errs = append(errs, err)
	}

	redisPort, err := getIntDefault("REDIS_PORT", 6379)
	if err != nil {
		errs = append(errs, err)
	} else if err := checkPort(redisPort); err != nil {
		errs = append(errs, err)
	}
	rateLimitPerIP, err := getIntDefault("RATE_LIMIT_PER_IP", 100)
	if err != nil {
		errs = append(errs, err)
	} else if rateLimitPerIP <= 0 {
		errs = append(errs, fmt.Errorf("RATE_LIMIT_PER_IP must be positive"))
	}

	maxRequestBodyBytes, err := getInt64Default("MAX_REQUEST_BODY_BYTES", 1*1024*1024)
	if err != nil {
		errs = append(errs, err)
	} else if maxRequestBodyBytes <= 0 {
		errs = append(errs, fmt.Errorf("MAX_REQUEST_BODY_BYTES must be positive"))
	}

	upstreamTimeoutSeconds, err := getIntDefault("UPSTREAM_TIMEOUT_SECONDS", 30)
	if err != nil {
		errs = append(errs, err)
	} else if upstreamTimeoutSeconds <= 0 || int64(upstreamTimeoutSeconds) > int64((1<<63-1)/time.Second) {
		errs = append(errs, fmt.Errorf("UPSTREAM_TIMEOUT_SECONDS must be positive and fit a time.Duration"))
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &Config{
		HTTPPort:               httpPort,
		RedisHost:              envStr("REDIS_HOST", "localhost"),
		RedisPort:              redisPort,
		RateLimitPerIP:         rateLimitPerIP,
		MaxRequestBodyBytes:    maxRequestBodyBytes,
		UpstreamTimeoutSeconds: upstreamTimeoutSeconds,
		AuthHTTPAddress:        envStr("AUTH_HTTP_ADDRESS", "http://localhost:8081"),
		ProductHTTPAddress:     envStr("PRODUCT_HTTP_ADDRESS", "http://localhost:8082"),
		CartHTTPAddress:        envStr("CART_HTTP_ADDRESS", "http://localhost:8083"),
		InventoryHTTPAddress:   envStr("INVENTORY_HTTP_ADDRESS", "http://localhost:8084"),
		OrderHTTPAddress:       envStr("ORDER_HTTP_ADDRESS", "http://localhost:8085"),
		PaymentHTTPAddress:     envStr("PAYMENT_HTTP_ADDRESS", "http://localhost:8086"),
	}, nil
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func getInt(env string) (int, error) {
	v := os.Getenv(env)
	if v == "" {
		return 0, fmt.Errorf("%s: environment variable is not set", env)
	}

	i, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", env, v)
	}

	return i, nil
}

func checkPort(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("%d is not a valid port: it must be between 1 and 65535", port)
	}
	return nil
}

// Routes maps the documented API path prefixes to their owning services.
func (c *Config) Routes() map[string]string {
	return map[string]string{
		"/api/v1/auth":            c.AuthHTTPAddress,
		"/api/v1/products":        c.ProductHTTPAddress,
		"/api/v1/admin/products":  c.ProductHTTPAddress,
		"/api/v1/cart":            c.CartHTTPAddress,
		"/api/v1/admin/inventory": c.InventoryHTTPAddress,
		"/api/v1/checkout":        c.OrderHTTPAddress,
		"/api/v1/orders":          c.OrderHTTPAddress,
		"/api/v1/webhooks/stripe": c.PaymentHTTPAddress,
	}
}

func getIntDefault(env string, fallback int) (int, error) {
	if os.Getenv(env) == "" {
		return fallback, nil
	}
	return getInt(env)
}

func (c *Config) RedisAddr() string {
	return net.JoinHostPort(c.RedisHost, strconv.Itoa(c.RedisPort))
}

func getInt64Default(env string, fallback int64) (int64, error) {
	v := os.Getenv(env)
	if v == "" {
		return fallback, nil
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", env, v)
	}
	return i, nil
}
