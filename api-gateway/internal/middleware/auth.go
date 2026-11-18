package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	grpcadapter "ecommerce.local/api-gateway/internal/adapter/grpc"
)

type userIDKey struct{}
type roleKey struct{}

// Auth delegates token verification to Auth and overwrites external identity headers.
func Auth(client *grpcadapter.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Del("X-User-ID")
			r.Header.Del("X-User-Role")
			if isPublicPath(r) {
				next.ServeHTTP(w, r)
				return
			}

			headers := r.Header.Values("Authorization")
			if len(headers) != 1 {
				writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid bearer token")
				return
			}
			parts := strings.Fields(headers[0])
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid bearer token")
				return
			}
			response, err := client.ValidateToken(r.Context(), parts[1], CorrelationFromCtx(r.Context()))
			if err != nil {
				slog.Warn("Auth token verification unavailable", "error", err, "correlation_id", CorrelationFromCtx(r.Context()), "service", "api-gateway")
				writeAuthError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication service unavailable")
				return
			}
			if !response.Valid {
				writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "token is invalid or expired")
				return
			}
			if response.UserId == "" || (response.Role != "Customer" && response.Role != "Administrator") {
				writeAuthError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication service unavailable")
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey{}, response.UserId)
			ctx = context.WithValue(ctx, roleKey{}, response.Role)
			r = r.WithContext(ctx)
			r.Header.Set("X-User-ID", response.UserId)
			r.Header.Set("X-User-Role", response.Role)
			r.Header.Del("Authorization")
			next.ServeHTTP(w, r)
		})
	}
}

// Public routes are deliberately method-specific; other business requests require verification.
func isPublicPath(r *http.Request) bool {
	path := r.URL.Path
	if r.Method == http.MethodPost {
		switch path {
		case "/api/v1/auth/register", "/api/v1/auth/login", "/api/v1/webhooks/stripe":
			return true
		}
	}
	if r.Method == http.MethodGet {
		switch path {
		case "/health/liveness", "/health/readiness", "/metrics", "/api/v1/products":
			return true
		}
		const prefix = "/api/v1/products/"
		if strings.HasPrefix(path, prefix) {
			id := strings.TrimPrefix(path, prefix)
			return id != "" && !strings.Contains(id, "/")
		}
	}
	return false
}

func UserIDFromCtx(ctx context.Context) string {
	value, _ := ctx.Value(userIDKey{}).(string)
	return value
}
func RoleFromCtx(ctx context.Context) string { value, _ := ctx.Value(roleKey{}).(string); return value }

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":   nil,
		"errors": []map[string]string{{"error_code": code, "message": message}},
	})
}
