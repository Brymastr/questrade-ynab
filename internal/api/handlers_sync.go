package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/brymastr/questrade-ynab/internal/db"
	"github.com/brymastr/questrade-ynab/internal/questrade"
	appsync "github.com/brymastr/questrade-ynab/internal/sync"
)

func handleRunSync(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)

		// dry_run computes deltas and the transactions that would be created
		// without writing anything to YNAB (used by the preview step).
		dryRun := r.URL.Query().Get("dry_run") == "true"

		mappings, err := store.GetMappings(userID)
		if err != nil || len(mappings) == 0 {
			writeError(w, http.StatusBadRequest, "no mappings configured")
			return
		}

		// Collect distinct budgetID → ynab access token
		ynabTokens := make(map[string]string)
		tok, _ := store.GetToken(userID, "ynab")
		if tok == nil {
			writeError(w, http.StatusBadRequest, "ynab not connected")
			return
		}
		for _, m := range mappings {
			ynabTokens[m.YNABBudgetID] = tok.AccessToken
		}

		var syncMappings []appsync.Mapping
		for _, m := range mappings {
			syncMappings = append(syncMappings, appsync.Mapping{
				QuestradeAccountNumber: m.QuestradeAccountNumber,
				YNABBudgetID:           m.YNABBudgetID,
				YNABAccountID:          m.YNABAccountID,
			})
		}

		var result *appsync.Result
		err = withQuestradeClient(store, userID, func(qc *questrade.Client) error {
			res, e := appsync.Run(qc, ynabTokens, syncMappings, dryRun)
			if e != nil {
				return e
			}
			result = res
			return nil
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// A preview doesn't change anything, so don't record it in history.
		if !dryRun {
			status := "success"
			if result.Errors > 0 && result.AccountsSynced == 0 {
				status = "error"
			} else if result.Errors > 0 {
				status = "partial"
			}

			_ = store.CreateSyncHistory(db.SyncHistory{
				UserID:         userID,
				RanAt:          time.Now(),
				Status:         status,
				AccountsSynced: result.AccountsSynced,
				Detail:         appsync.MarshalDetail(result.AccountResults),
			})
		}

		writeJSON(w, http.StatusOK, result)
	}
}

func handleGetSchedule(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)
		sc, err := store.GetSchedule(userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, sc)
	}
}

func handlePutSchedule(store db.Store, sched ScheduleManager) http.HandlerFunc {
	type scheduleInput struct {
		CronExpression string `json:"cron_expression"`
		Enabled        bool   `json:"enabled"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)

		var inp scheduleInput
		if err := json.NewDecoder(r.Body).Decode(&inp); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if inp.CronExpression == "" {
			writeError(w, http.StatusBadRequest, "cron_expression required")
			return
		}

		sc := db.SyncSchedule{
			UserID:         userID,
			CronExpression: inp.CronExpression,
			Enabled:        inp.Enabled,
		}
		if err := store.UpsertSchedule(sc); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Update the live scheduler
		if inp.Enabled {
			sched.Register(userID, inp.CronExpression)
		} else {
			sched.Deregister(userID)
		}

		saved, _ := store.GetSchedule(userID)
		writeJSON(w, http.StatusOK, saved)
	}
}

func handleGetSyncHistory(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}
		history, err := store.GetSyncHistory(userID, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if history == nil {
			history = []db.SyncHistory{}
		}
		writeJSON(w, http.StatusOK, history)
	}
}
