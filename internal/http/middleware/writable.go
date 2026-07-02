package middleware

import (
	"net/http"
	"veloxmesh/internal/coordination"
)

// RequireWritable ensures that the node is writable before allowing the request to proceed.
func RequireWritable(coord coordination.Coordinator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				if coord != nil && !coord.IsWritable() {
					http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
