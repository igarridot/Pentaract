package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// redactedRequestURI is the request line for logs with credential-bearing
// query values masked. Downloads authenticate with ?access_token=, which must
// never end up in the access log.
func redactedRequestURI(u *url.URL) string {
	if u.RawQuery == "" {
		return u.RequestURI()
	}
	query := u.Query()
	if _, ok := query["access_token"]; ok {
		query.Set("access_token", "REDACTED")
	}
	return u.EscapedPath() + "?" + query.Encode()
}

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
			"request", fmt.Sprintf("%s %s %s", r.Method, redactedRequestURI(r.URL), r.Proto),
			"from", r.RemoteAddr,
			"status", status,
			"bytes", ww.BytesWritten(),
			"duration", time.Since(startedAt),
			"user_agent", r.UserAgent(),
		)
	})
}
