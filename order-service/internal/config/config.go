package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPPort                int
	GRPCPort                int
	PostgresDSN             string
	CartGRPCAddress         string
	ProductGRPCAddress      string
	InventoryGRPCAddress    string
	RPCTimeoutSeconds       int
	CheckoutTimeoutSeconds  int
	RecoveryIntervalSeconds int
	RecoveryBatchSize       int
}

func Load() (*Config, error) {
	var errs []error

	httpPort, err := getInt("HTTP_PORT")
	if err != nil {
		errs = append(errs, err)
	} else if err := checkPort(httpPort); err != nil {
		errs = append(errs, err)
	}

	grpcPort, err := getInt("GRPC_PORT")
	if err != nil {
		errs = append(errs, err)
	} else if err := checkPort(grpcPort); err != nil {
		errs = append(errs, err)
	}

	postgresDSN, err := getStr("POSTGRES_DSN")
	if err != nil {
		errs = append(errs, err)
	}

	cartAddress, err := getStr("CART_GRPC_ADDRESS")
	if err != nil {
		errs = append(errs, err)
	}
	productAddress, err := getStr("PRODUCT_GRPC_ADDRESS")
	if err != nil {
		errs = append(errs, err)
	}
	inventoryAddress, err := getStr("INVENTORY_GRPC_ADDRESS")
	if err != nil {
		errs = append(errs, err)
	}
	rpcTimeout, err := getTimeout("RPC_TIMEOUT_SECONDS", 5)
	if err != nil {
		errs = append(errs, err)
	}
	checkoutTimeout, err := getTimeout("CHECKOUT_TIMEOUT_SECONDS", 20)
	if err != nil {
		errs = append(errs, err)
	}

	recoveryInterval, err := getTimeout("RECOVERY_INTERVAL_SECONDS", 5)
	if err != nil {
		errs = append(errs, err)
	}
	recoveryBatchSize := 10
	if os.Getenv("RECOVERY_BATCH_SIZE") != "" {
		recoveryBatchSize, err = getInt("RECOVERY_BATCH_SIZE")
		if err != nil {
			errs = append(errs, err)
		}
	}
	if recoveryBatchSize < 1 || recoveryBatchSize > 100 {
		errs = append(errs, fmt.Errorf("RECOVERY_BATCH_SIZE must be between 1 and 100"))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &Config{
		HTTPPort:                httpPort,
		GRPCPort:                grpcPort,
		PostgresDSN:             postgresDSN,
		CartGRPCAddress:         cartAddress,
		ProductGRPCAddress:      productAddress,
		InventoryGRPCAddress:    inventoryAddress,
		RPCTimeoutSeconds:       rpcTimeout,
		CheckoutTimeoutSeconds:  checkoutTimeout,
		RecoveryIntervalSeconds: recoveryInterval,
		RecoveryBatchSize:       recoveryBatchSize,
	}, nil
}

func getStr(env string) (string, error) {
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("%s: environment variable is not set", env)
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

func getTimeout(env string, fallback int) (int, error) {
	if os.Getenv(env) == "" {
		return fallback, nil
	}
	value, err := getInt(env)
	if err != nil {
		return 0, err
	}
	if value <= 0 || value > 3600 {
		return 0, fmt.Errorf("%s must be between 1 and 3600 seconds", env)
	}
	return value, nil
}
