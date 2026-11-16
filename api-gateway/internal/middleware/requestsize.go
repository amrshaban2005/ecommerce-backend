package middleware

import (
	"encoding/json"
	"net/http"
)

// RequestSizeLimit rejects requests whose body exceeds maxBytes.
func RequestSizeLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data":   nil,
					"errors": []map[string]string{{"error_code": "REQUEST_TOO_LARGE", "message": "request body exceeds size limit"}},
				})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
