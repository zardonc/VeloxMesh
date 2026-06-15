package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type responseWriterObserver struct {
	http.ResponseWriter
	status int
}

func (w *responseWriterObserver) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		obs := &responseWriterObserver{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(obs, r)

		duration := time.Since(start)
		reqID := GetReqID(r.Context())

		slog.Info("request completed",
			"request_id", reqID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", obs.status,
			"latency_ms", duration.Milliseconds(),
		)
	})
}
