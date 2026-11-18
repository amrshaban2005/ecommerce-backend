package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	HTTPPort    int
	GRPCPort    int
	PostgresDSN string
	JWTSecret   string
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

	jwtSecret, err := getStr("JWT_SECRET")
	if err != nil {
		errs = append(errs, err)
	} else if len(jwtSecret) < 32 {
		errs = append(errs, fmt.Errorf("JWT_SECRET must contain at least 32 bytes"))
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &Config{
		HTTPPort:    httpPort,
		GRPCPort:    grpcPort,
		PostgresDSN: postgresDSN,
		JWTSecret:   jwtSecret,
	}, nil
}

func getStr(env string) (string, error) {
	if v := os.Getenv(env); v != "" {
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
