package api

import (
	"net/http"
	"os"

	"github.com/brymastr/questrade-ynab/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// ScheduleManager is the interface the sync handler uses to update live cron jobs.
// Implemented by *scheduler.Scheduler.
type ScheduleManager interface {
	Register(userID, cronExpr string)
	Deregister(userID string)
}

func NewRouter(store db.Store, sched ScheduleManager) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{corsOrigin()},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// OAuth — public
	// Questrade personal apps can't use the redirect flow, so the client posts a
	// manually-generated refresh token here rather than being redirected out.
	r.Post("/auth/questrade", handleQuestradeConnect(store))
	r.Get("/auth/ynab", handleYNABLogin(store))
	r.Get("/auth/ynab/callback", handleYNABCallback(store))
	r.Post("/auth/logout", handleLogout)

	// API — requires session
	r.Group(func(r chi.Router) {
		r.Use(requireAuth)

		r.Get("/api/me", handleMe(store))

		r.Get("/api/accounts/questrade", handleQuestradeAccounts(store))
		r.Get("/api/budgets/ynab", handleYNABBudgets(store))
		r.Get("/api/accounts/ynab", handleYNABAccounts(store))

		r.Get("/api/mappings", handleGetMappings(store))
		r.Put("/api/mappings", handlePutMappings(store))
		r.Delete("/api/mappings/{id}", handleDeleteMapping(store))

		r.Post("/api/sync/run", handleRunSync(store))
		r.Get("/api/sync/schedule", handleGetSchedule(store))
		r.Put("/api/sync/schedule", handlePutSchedule(store, sched))
		r.Get("/api/sync/history", handleGetSyncHistory(store))
	})

	return r
}

func corsOrigin() string {
	if o := os.Getenv("CORS_ORIGIN"); o != "" {
		return o
	}
	return "http://localhost:5173"
}
