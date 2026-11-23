package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPPort             int
	GRPCPort             int
	PostgresDSN          string
	StripeSecretKey      string
	StripeWebhookSecret  string
	RabbitMQURL          string
	SuccessURL           string
	CancelURL            string
	StripeTimeoutSeconds int
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

	secret, err := getStr("STRIPE_SECRET_KEY")
	if err != nil {
		errs = append(errs, err)
	} else if !strings.HasPrefix(secret, "sk_test_") {
		errs = append(errs, fmt.Errorf("STRIPE_SECRET_KEY must be a Stripe test secret key"))
	}
	webhookSecret, err := getStr("STRIPE_WEBHOOK_SECRET")
	if err != nil {
		errs = append(errs, err)
	} else if !strings.HasPrefix(webhookSecret, "whsec_") {
		errs = append(errs, fmt.Errorf("STRIPE_WEBHOOK_SECRET must be a webhook signing secret"))
	}
	rabbitMQURL, err := getStr("RABBITMQ_URL")
	if err != nil {
		errs = append(errs, err)
	} else if parsed, parseErr := url.Parse(rabbitMQURL); parseErr != nil || (parsed.Scheme != "amqp" && parsed.Scheme != "amqps") || parsed.Hostname() == "" {
		errs = append(errs, fmt.Errorf("RABBITMQ_URL must be an absolute AMQP(S) URL"))
	}
	successURL, err := getStr("PAYMENT_SUCCESS_URL")
	if err != nil {
		errs = append(errs, err)
	} else if !validURL(successURL) {
		errs = append(errs, fmt.Errorf("PAYMENT_SUCCESS_URL must be an absolute HTTP(S) URL without credentials or fragment"))
	}
	cancelURL, err := getStr("PAYMENT_CANCEL_URL")
	if err != nil {
		errs = append(errs, err)
	} else if !validURL(cancelURL) {
		errs = append(errs, fmt.Errorf("PAYMENT_CANCEL_URL must be an absolute HTTP(S) URL without credentials or fragment"))
	}
	timeout := 5
	if os.Getenv("STRIPE_TIMEOUT_SECONDS") != "" {
		timeout, err = getInt("STRIPE_TIMEOUT_SECONDS")
		if err != nil {
			errs = append(errs, err)
		}
	}
	if timeout < 1 || timeout > 60 {
		errs = append(errs, fmt.Errorf("STRIPE_TIMEOUT_SECONDS must be between 1 and 60"))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &Config{
		HTTPPort:             httpPort,
		GRPCPort:             grpcPort,
		PostgresDSN:          postgresDSN,
		StripeSecretKey:      secret,
		StripeWebhookSecret:  webhookSecret,
		RabbitMQURL:          rabbitMQURL,
		SuccessURL:           successURL,
		CancelURL:            cancelURL,
		StripeTimeoutSeconds: timeout,
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

func validURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == ""
}
