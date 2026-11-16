package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// RequestLogger logs every request with structured JSON fields.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		corrID := CorrelationFromCtx(r.Context())

		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"correlation_id", corrID,
			"service", "api-gateway",
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// Flush delegates to the underlying ResponseWriter's Flusher, if it has one.
// Wrapping http.ResponseWriter in a plain struct hides optional interfaces
// like http.Flusher unless explicitly forwarded -- needed for streaming
// handlers (e.g. SSE) that sit behind this middleware.
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
