package handler

import (
	"context"
	"net/http"
	"strings"

	appjwt "github.com/Dominux/Pentaract/internal/jwt"
)

type contextKey string

const authUserKey contextKey = "auth_user"

func AuthMiddleware(secretKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")

			token := ""
			if header != "" {
				parts := strings.SplitN(header, " ", 2)
				if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
					http.Error(w, `{"error":"invalid authorization header"}`, http.StatusUnauthorized)
					return
				}
				token = parts[1]
			} else {
				token = r.URL.Query().Get("access_token")
				if token == "" {
					http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
					return
				}
				if !strings.Contains(r.URL.Path, "/files/download/") && !strings.Contains(r.URL.Path, "/files/download_dir/") {
					http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
					return
				}
			}

			user, err := appjwt.Validate(token, secretKey)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), authUserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetAuthUser(ctx context.Context) *appjwt.AuthUser {
	user, _ := ctx.Value(authUserKey).(*appjwt.AuthUser)
	return user
}
