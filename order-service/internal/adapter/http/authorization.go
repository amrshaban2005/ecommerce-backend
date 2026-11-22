package httpadapter

import (
	"net/http"
	"strings"
)

// RequireCustomer trusts identity headers supplied by the protected gateway.
// The gateway removes client-supplied identity and verifies the token through Auth.
func RequireCustomer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userIDs := r.Header.Values("X-User-ID")
		roles := r.Header.Values("X-User-Role")
		if len(userIDs) != 1 || strings.TrimSpace(userIDs[0]) == "" ||
			len(roles) != 1 || strings.TrimSpace(roles[0]) == "" {
			respondJSON(w, http.StatusUnauthorized, envelope{
				Data:   nil,
				Errors: []apiError{{Code: "UNAUTHORIZED", Message: "verified identity is required"}},
			})
			return
		}
		if roles[0] != "Customer" {
			respondJSON(w, http.StatusForbidden, envelope{
				Data:   nil,
				Errors: []apiError{{Code: "FORBIDDEN", Message: "customer permission is required"}},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
