package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/brymastr/questrade-ynab/internal/questrade"
	"github.com/brymastr/ynab-go"
)

// Mapping pairs a Questrade account to a YNAB account within a budget.
type Mapping struct {
	QuestradeAccountNumber string
	YNABBudgetID           string
	YNABAccountID          string
}

// AccountResult describes the outcome of syncing one account pair.
type AccountResult struct {
	QuestradeNumber string  `json:"questrade_number"`
	QuestradeType   string  `json:"questrade_type"`
	YNABName        string  `json:"ynab_name"`
	OldBalance      float64 `json:"old_balance"`
	NewBalance      float64 `json:"new_balance"`
	Delta           float64 `json:"delta"`
	Error           string  `json:"error,omitempty"`
	// AlreadySynced is true when YNAB already had this adjustment (duplicate
	// import_id), so nothing new was created.
	AlreadySynced bool `json:"already_synced,omitempty"`
}

// Result is the aggregate output of a sync run.
type Result struct {
	AccountResults []AccountResult `json:"account_results"`
	AccountsSynced int             `json:"accounts_synced"`
	Errors         int             `json:"errors"`
}

// Run fetches live balances from Questrade, computes deltas against YNAB, and
// creates adjustment transactions. Set dryRun=true to skip transaction creation.
func Run(qClient *questrade.Client, ynabTokens map[string]string, mappings []Mapping, dryRun bool) (*Result, error) {
	// Ensure Questrade access token is valid
	if !qClient.IsTokenValid() {
		if _, err := qClient.Refresh(); err != nil {
			return nil, fmt.Errorf("questrade token refresh: %w", err)
		}
	}

	ctx := context.Background()

	qAccounts, err := qClient.GetAccounts()
	if err != nil {
		return nil, fmt.Errorf("fetch questrade accounts: %w", err)
	}
	qByNumber := make(map[string]*questrade.Account, len(qAccounts))
	for i := range qAccounts {
		qByNumber[qAccounts[i].Number] = &qAccounts[i]
	}

	// ynabTokens: budgetID → accessToken (callers pass one entry per distinct budget)
	// Pre-build ynab clients keyed by budgetID (the plan ID is passed per call)
	ynabClients := make(map[string]*ynab.Client, len(ynabTokens))
	for budgetID, token := range ynabTokens {
		ynabClients[budgetID] = ynab.NewClient(token, ynab.WithUserAgent("questrade-ynab"))
	}

	// Fetch all YNAB accounts per budget (deduplicated)
	type ynabKey struct{ budgetID, accountID string }
	ynabAccounts := make(map[ynabKey]*ynab.Account)
	for budgetID, yc := range ynabClients {
		accs, err := yc.GetAccounts(ctx, budgetID)
		if err != nil {
			return nil, fmt.Errorf("fetch ynab accounts for budget %s: %w", budgetID, err)
		}
		for i := range accs {
			k := ynabKey{budgetID, accs[i].ID}
			acc := accs[i]
			ynabAccounts[k] = &acc
		}
	}

	result := &Result{}
	today := ynab.NewDate(time.Now())

	for _, m := range mappings {
		ar := AccountResult{QuestradeNumber: m.QuestradeAccountNumber}

		qAcc, ok := qByNumber[m.QuestradeAccountNumber]
		if !ok || qAcc.Balances == nil || len(qAcc.Balances.CombinedBalances) == 0 {
			ar.Error = "questrade account or balance not found"
			result.AccountResults = append(result.AccountResults, ar)
			result.Errors++
			log.Printf("sync: skipping %s — %s", m.QuestradeAccountNumber, ar.Error)
			continue
		}
		ar.QuestradeType = qAcc.Type
		ar.NewBalance = qAcc.Balances.CombinedBalances[0].TotalEquity

		yAcc, ok := ynabAccounts[ynabKey{m.YNABBudgetID, m.YNABAccountID}]
		if !ok {
			ar.Error = fmt.Sprintf("ynab account %s not found in budget %s", m.YNABAccountID, m.YNABBudgetID)
			result.AccountResults = append(result.AccountResults, ar)
			result.Errors++
			log.Printf("sync: skipping %s — %s", m.QuestradeAccountNumber, ar.Error)
			continue
		}
		ar.YNABName = yAcc.Name
		// Whole-cent milliunits avoid float residue (e.g. -1e-13) that would
		// show as "+0"/"-0" and create bogus ~$0 transactions.
		oldMilli, newMilli, deltaMilliunits := PlanAdjustment(yAcc.Balance, ar.NewBalance)
		ar.OldBalance = float64(oldMilli) / 1000
		ar.NewBalance = float64(newMilli) / 1000
		ar.Delta = float64(deltaMilliunits) / 1000

		if deltaMilliunits == 0 {
			result.AccountResults = append(result.AccountResults, ar)
			continue
		}

		if !dryRun {
			yc := ynabClients[m.YNABBudgetID]
			importID := AdjustmentImportID(m.YNABAccountID, today, oldMilli, newMilli)
			tx := ynab.NewTransaction{
				AccountID: m.YNABAccountID,
				Date:      today,
				Amount:    ynab.Milliunits(deltaMilliunits),
				PayeeName: "Stock Market",
				Memo:      "Questrade sync",
				Cleared:   ynab.Cleared,
				Approved:  true,
				ImportID:  importID,
			}
			if res, err := yc.CreateTransactions(ctx, m.YNABBudgetID, []ynab.NewTransaction{tx}); err != nil {
				ar.Error = fmt.Sprintf("create transaction: %v", err)
				result.Errors++
				log.Printf("sync: error creating transaction for %s: %v", m.QuestradeAccountNumber, err)
			} else if IsDuplicateImportID(res, importID) {
				ar.AlreadySynced = true
				log.Printf("sync: adjustment for %s already in YNAB (duplicate import_id), skipping", m.QuestradeAccountNumber)
			} else {
				result.AccountsSynced++
			}
		} else {
			result.AccountsSynced++
		}

		result.AccountResults = append(result.AccountResults, ar)
	}

	return result, nil
}

// MarshalDetail serializes AccountResults to a JSON string for storage.
func MarshalDetail(results []AccountResult) string {
	b, _ := json.Marshal(results)
	return string(b)
}
