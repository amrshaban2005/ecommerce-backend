package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// CorrelationHeader is the HTTP header carrying the correlation ID.
const CorrelationHeader = "X-Correlation-Id"

type corrKey struct{}

// CorrelationIDMiddleware extracts X-Correlation-Id from inbound requests.
// If not present, generates a new UUID v7.
func CorrelationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get(CorrelationHeader)
		if corrID == "" {
			id, err := uuid.NewV7()
			if err != nil {
				id = uuid.New() // fallback to v4 only if v7 fails (extremely unlikely)
			}
			corrID = id.String()
		}

		// Echo back on response
		w.Header().Set(CorrelationHeader, corrID)

		ctx := context.WithValue(r.Context(), corrKey{}, corrID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CorrelationFromCtx retrieves correlation_id from context.
func CorrelationFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(corrKey{}).(string)
	return id
}
