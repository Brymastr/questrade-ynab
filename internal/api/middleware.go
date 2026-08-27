package api

import (
	"context"
	"net/http"

	"github.com/brymastr/questrade-ynab/internal/auth"
)

type contextKey string

const userIDKey contextKey = "userID"

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := auth.GetSession(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFromCtx(r *http.Request) string {
	v, _ := r.Context().Value(userIDKey).(string)
	return v
}
