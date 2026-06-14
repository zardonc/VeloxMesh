package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"veloxmesh/internal/errors"
)

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				reqID := GetReqID(r.Context())
				slog.Error("panic recovered",
					"request_id", reqID,
					"error", err,
					"stack", string(debug.Stack()),
				)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(errors.NewGatewayError("internal_error", "Internal server error", http.StatusInternalServerError))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
