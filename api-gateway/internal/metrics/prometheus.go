// Package metrics provides the Prometheus metrics handler and custom metric registration.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Gateway-level Prometheus metrics.
var (
	// RequestsTotal counts HTTP requests by method, path and status code.
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ecommerce_gateway_requests_total",
			Help: "Total number of HTTP requests processed by the gateway.",
		},
		[]string{"method", "path", "status_code"},
	)

	// RequestDurationSeconds tracks request latency per route.
	RequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ecommerce_gateway_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// RateLimitedTotal counts rate-limited requests.
	RateLimitedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "ecommerce_gateway_rate_limited_total",
			Help: "Total number of rate-limited requests.",
		},
	)

	// CircuitBreakerState tracks open circuit breakers per service.
	CircuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ecommerce_gateway_circuit_breaker_open",
			Help: "1 if circuit breaker is open for a service, 0 otherwise.",
		},
		[]string{"service"},
	)
)

// Handler returns the Prometheus metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}
