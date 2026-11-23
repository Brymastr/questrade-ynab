package api

import (
	"context"
	"net/http"

	"github.com/brymastr/questrade-ynab/internal/auth"
	"github.com/brymastr/questrade-ynab/internal/db"
	"github.com/go-chi/chi/v5/middleware"
	"log"
	"time"
)

type contextKey string

const userIDKey contextKey = "userID"

func requireAuth(store db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return requireAuthWith(store, next)
	}
}

func requireAuthWith(store db.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := auth.GetSession(r, store)
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

// requestLogger logs method, path, status, size and duration. Unlike chi's
// middleware.Logger it leaves out the query string (the YNAB OAuth callback
// carries an authorization code there) and the client address.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		log.Printf("%s %s %d %dB %s", r.Method, r.URL.Path, ww.Status(), ww.BytesWritten(), time.Since(start).Round(time.Microsecond))
	})
}
