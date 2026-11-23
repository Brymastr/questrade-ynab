package scheduler

import (
	"errors"
	"log"
	"time"

	"github.com/brymastr/questrade-ynab/internal/auth"
	"github.com/brymastr/questrade-ynab/internal/db"
	"github.com/brymastr/questrade-ynab/internal/questrade"
	appsync "github.com/brymastr/questrade-ynab/internal/sync"
	"github.com/robfig/cron/v3"
)

// Scheduler manages per-user cron jobs that trigger account syncs.
type Scheduler struct {
	cron    *cron.Cron
	store   db.Store
	entries map[string]cron.EntryID // userID → cron entry
}

func New(store db.Store) *Scheduler {
	return &Scheduler{
		cron:    cron.New(),
		store:   store,
		entries: make(map[string]cron.EntryID),
	}
}

// Start begins the cron engine and loads all enabled schedules from the DB.
func (s *Scheduler) Start() {
	schedules, err := s.store.GetAllEnabledSchedules()
	if err != nil {
		log.Printf("scheduler: failed to load schedules: %v", err)
	} else {
		for _, sc := range schedules {
			s.Register(sc.UserID, sc.CronExpression)
		}
	}
	s.cron.Start()
	log.Printf("scheduler: started with %d active jobs", len(s.entries))
}

// Stop gracefully shuts down the cron engine.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// Register adds (or replaces) a cron job for the given user.
func (s *Scheduler) Register(userID, cronExpr string) {
	s.Deregister(userID) // remove existing job if present

	entryID, err := s.cron.AddFunc(cronExpr, func() {
		s.runSyncForUser(userID)
	})
	if err != nil {
		log.Printf("scheduler: invalid cron expression %q for user %s: %v", cronExpr, userID, err)
		return
	}
	s.entries[userID] = entryID
	log.Printf("scheduler: registered job for user %s with expression %q", userID, cronExpr)
}

// Deregister removes the cron job for the given user, if any.
func (s *Scheduler) Deregister(userID string) {
	if id, ok := s.entries[userID]; ok {
		s.cron.Remove(id)
		delete(s.entries, userID)
		log.Printf("scheduler: deregistered job for user %s", userID)
	}
}

func (s *Scheduler) runSyncForUser(userID string) {
	RunForUser(s.store, userID)
}

// DueUsers returns the users whose enabled schedule fires in the window
// (from, to]. It's for an external trigger (EventBridge Scheduler on Lambda)
// that ticks at a fixed interval instead of keeping an in-process cron; the
// caller decides how to run them (on Lambda, one SQS message each).
func DueUsers(store db.Store, from, to time.Time) ([]string, error) {
	schedules, err := store.GetAllEnabledSchedules()
	if err != nil {
		return nil, err
	}
	var due []string
	for _, sc := range schedules {
		ok, err := isDue(sc.CronExpression, from, to)
		if err != nil {
			log.Printf("scheduler: invalid cron expression %q for user %s: %v", sc.CronExpression, sc.UserID, err)
			continue
		}
		if ok {
			due = append(due, sc.UserID)
		}
	}
	log.Printf("scheduler: window (%s, %s]: %d of %d schedules due",
		from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339), len(due), len(schedules))
	return due, nil
}

// RunForUser syncs one user's mapped accounts, refreshing tokens as needed,
// and records the outcome in their sync history.
func RunForUser(store db.Store, userID string) {
	log.Printf("scheduler: running sync for user %s", userID)

	mappings, err := store.GetMappings(userID)
	if err != nil || len(mappings) == 0 {
		log.Printf("scheduler: no mappings for user %s, skipping", userID)
		return
	}

	qtTok, err := store.GetToken(userID, "questrade")
	if err != nil || qtTok == nil {
		log.Printf("scheduler: questrade not connected for user %s", userID)
		return
	}
	ynabTok, err := store.GetToken(userID, "ynab")
	if err != nil || ynabTok == nil {
		log.Printf("scheduler: ynab not connected for user %s", userID)
		return
	}

	// Build Questrade client, refreshing token if expired
	qc := questrade.NewClientFromTokens(qtTok.RefreshToken, qtTok.AccessToken, qtTok.APIServer, qtTok.ExpiresAt)
	if !qc.IsTokenValid() {
		res, err := auth.RefreshQuestradeToken(qtTok.RefreshToken)
		if err != nil {
			log.Printf("scheduler: questrade token refresh failed for user %s: %v", userID, err)
			return
		}
		qc = questrade.NewClientFromTokens(res.RefreshToken, res.AccessToken, res.APIServer, res.ExpiresAt)
		_ = store.UpsertToken(db.OAuthToken{
			UserID:       userID,
			Provider:     "questrade",
			AccessToken:  res.AccessToken,
			RefreshToken: res.RefreshToken,
			ExpiresAt:    res.ExpiresAt,
			APIServer:    res.APIServer,
		})
	}

	// Refresh YNAB token if needed
	if time.Now().After(ynabTok.ExpiresAt) {
		res, err := auth.RefreshYNABToken(ynabTok.RefreshToken)
		if err != nil {
			log.Printf("scheduler: ynab token refresh failed for user %s: %v", userID, err)
			return
		}
		_ = store.UpsertToken(db.OAuthToken{
			UserID:       userID,
			Provider:     "ynab",
			AccessToken:  res.AccessToken,
			RefreshToken: res.RefreshToken,
			ExpiresAt:    res.ExpiresAt,
		})
		ynabTok.AccessToken = res.AccessToken
	}

	// Collect distinct budget IDs → ynab access token
	ynabTokens := make(map[string]string)
	var syncMappings []appsync.Mapping
	for _, m := range mappings {
		ynabTokens[m.YNABBudgetID] = ynabTok.AccessToken
		syncMappings = append(syncMappings, appsync.Mapping{
			QuestradeAccountNumber: m.QuestradeAccountNumber,
			YNABBudgetID:           m.YNABBudgetID,
			YNABAccountID:          m.YNABAccountID,
		})
	}

	result, runErr := appsync.Run(qc, ynabTokens, syncMappings, false)

	// A locally-valid access token can still be rejected server-side (Questrade
	// rotates/invalidates tokens); force a refresh and retry once on a 401.
	if errors.Is(runErr, questrade.ErrUnauthorized) {
		if latest, gerr := store.GetToken(userID, "questrade"); gerr == nil && latest != nil {
			if res, rerr := auth.RefreshQuestradeToken(latest.RefreshToken); rerr == nil {
				qc = questrade.NewClientFromTokens(res.RefreshToken, res.AccessToken, res.APIServer, res.ExpiresAt)
				_ = store.UpsertToken(db.OAuthToken{
					UserID:       userID,
					Provider:     "questrade",
					AccessToken:  res.AccessToken,
					RefreshToken: res.RefreshToken,
					ExpiresAt:    res.ExpiresAt,
					APIServer:    res.APIServer,
				})
				result, runErr = appsync.Run(qc, ynabTokens, syncMappings, false)
			} else {
				log.Printf("scheduler: questrade re-refresh after 401 failed for user %s: %v", userID, rerr)
			}
		}
	}

	status := "success"
	if runErr != nil {
		status = "error"
		log.Printf("scheduler: sync error for user %s: %v", userID, runErr)
	} else if result.Errors > 0 && result.AccountsSynced == 0 {
		status = "error"
	} else if result.Errors > 0 {
		status = "partial"
	}

	accountsSynced := 0
	var detail string
	if result != nil {
		accountsSynced = result.AccountsSynced
		detail = appsync.MarshalDetail(result.AccountResults)
	}

	_ = store.CreateSyncHistory(db.SyncHistory{
		UserID:         userID,
		RanAt:          time.Now(),
		Status:         status,
		AccountsSynced: accountsSynced,
		Detail:         detail,
	})
}
