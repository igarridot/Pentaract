package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		startedAt := time.Now()

		next.ServeHTTP(ww, r)

		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}

		slog.Info(
			"http request",
			"request", fmt.Sprintf("%s %s %s", r.Method, r.URL.RequestURI(), r.Proto),
			"from", r.RemoteAddr,
			"status", status,
			"bytes", ww.BytesWritten(),
			"duration", time.Since(startedAt),
			"user_agent", r.UserAgent(),
		)
	})
}
