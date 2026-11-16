// Package proxy routes HTTP requests to configured upstream services.
package proxy

import (
	"context"
	"ecommerce.local/api-gateway/internal/circuit"
	"ecommerce.local/api-gateway/internal/middleware"
	"ecommerce.local/api-gateway/internal/ratelimit"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Router struct {
	routes          map[string]*route
	limiter         *ratelimit.Limiter
	upstreamTimeout time.Duration
}

type options struct {
	circuitRecoveryWait time.Duration
	upstreamTimeout     time.Duration
}

// Option configures Router construction.
type Option func(*options)

// WithCircuitRecoveryWait overrides the per-route circuit breaker recovery window.
func WithCircuitRecoveryWait(wait time.Duration) Option {
	return func(opts *options) {
		if wait > 0 {
			opts.circuitRecoveryWait = wait
		}
	}
}

// WithUpstreamTimeout sets the deadline for each forwarded request.
func WithUpstreamTimeout(timeout time.Duration) Option {
	return func(opts *options) {
		if timeout > 0 {
			opts.upstreamTimeout = timeout
		}
	}
}

type route struct {
	prefix  string
	target  *url.URL
	breaker *circuit.Breaker
	proxy   *httputil.ReverseProxy
}

// New builds a prefix-to-upstream routing table. Request paths are preserved.
func New(routes map[string]string, limiter *ratelimit.Limiter, opts ...Option) (*Router, error) {
	cfg := options{upstreamTimeout: 30 * time.Second}
	for _, opt := range opts {
		opt(&cfg)
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("at least one upstream route is required")
	}
	r := &Router{routes: make(map[string]*route, len(routes)), limiter: limiter, upstreamTimeout: cfg.upstreamTimeout}
	for prefix, upstream := range routes {
		if !strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "?#") {
			return nil, fmt.Errorf("invalid route prefix %q", prefix)
		}
		target, err := url.Parse(upstream)
		if err != nil || target == nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
			return nil, fmt.Errorf("route %q requires an HTTP or HTTPS upstream without credentials, query or fragment", prefix)
		}

		var brk *circuit.Breaker
		if cfg.circuitRecoveryWait > 0 {
			brk = circuit.New(prefix, circuit.WithHalfOpenWait(cfg.circuitRecoveryWait))
		} else {
			brk = circuit.New(prefix)
		}
		proxy := newReverseProxy(target, brk)

		r.routes[prefix] = &route{
			prefix:  prefix,
			target:  target,
			breaker: brk,
			proxy:   proxy,
		}
	}
	return r, nil
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rt := r.findRoute(req.URL.Path)
	if rt == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":   nil,
			"errors": []map[string]string{{"error_code": "NOT_FOUND", "message": "no route matched"}},
		})
		return
	}

	// Check the limit before acquiring a circuit-breaker recovery probe.
	if r.limiter != nil {
		clientIP, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			clientIP = req.RemoteAddr
		}
		if ip := net.ParseIP(clientIP); ip != nil {
			clientIP = ip.String()
		}
		result := r.limiter.Check(req.Context(), clientIP)
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(result.ResetAfter).Unix(), 10))
		if !result.Allowed {
			slog.Warn("rate limit exceeded", "client_ip", clientIP, "limit", result.Limit, "correlation_id", middleware.CorrelationFromCtx(req.Context()), "service", "api-gateway")
			w.Header().Set("Retry-After", strconv.Itoa(int(result.ResetAfter.Seconds())))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":   nil,
				"errors": []map[string]string{{"error_code": "RATE_LIMITED", "message": "rate limit exceeded"}},
			})
			return
		}
	}

	// Circuit breaker check
	if !rt.breaker.Allow() {
		slog.Warn("circuit open, rejecting request",
			"prefix", rt.prefix,
			"path", req.URL.Path,
			"correlation_id", middleware.CorrelationFromCtx(req.Context()),
			"service", "api-gateway",
		)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":   nil,
			"errors": []map[string]string{{"error_code": "SERVICE_UNAVAILABLE", "message": "service temporarily unavailable"}},
		})
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), r.upstreamTimeout)
	defer cancel()
	rt.proxy.ServeHTTP(w, req.WithContext(ctx))
}

func (r *Router) findRoute(path string) *route {
	// Longest prefix match
	var matched *route
	var matchedLen int
	for prefix, rt := range r.routes {
		if matchesRoutePrefix(path, prefix) && len(prefix) > matchedLen {
			matched = rt
			matchedLen = len(prefix)
		}
	}
	return matched
}

func matchesRoutePrefix(path, prefix string) bool {
	if prefix == "/" {
		return strings.HasPrefix(path, "/")
	}
	if path == prefix {
		return true
	}
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	if strings.HasSuffix(prefix, "/") {
		return true
	}
	return len(path) > len(prefix) && path[len(prefix)] == '/'
}

func newReverseProxy(target *url.URL, brk *circuit.Breaker) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		if corr := middleware.CorrelationFromCtx(req.Context()); corr != "" {
			req.Header.Set(middleware.CorrelationHeader, corr)
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode >= 500 {
			brk.Failure()
		} else {
			brk.Success()
		}
		return nil
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		brk.Failure()
		slog.Error("proxy error",
			"target", target.String(),
			"path", req.URL.Path,
			"correlation_id", middleware.CorrelationFromCtx(req.Context()),
			"error", err,
			"service", "api-gateway",
		)
		w.Header().Set("Content-Type", "application/json")
		statusCode := http.StatusBadGateway
		code, message := "BAD_GATEWAY", "upstream service error"
		var timeoutError net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(req.Context().Err(), context.DeadlineExceeded) || (errors.As(err, &timeoutError) && timeoutError.Timeout()) {
			statusCode = http.StatusGatewayTimeout
			code, message = "GATEWAY_TIMEOUT", "upstream request timed out"
		}
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":   nil,
			"errors": []map[string]string{{"error_code": code, "message": message}},
		})
	}
	return proxy
}
