package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brymastr/questrade-ynab/internal/questrade"
	qysync "github.com/brymastr/questrade-ynab/internal/sync"
	"github.com/brymastr/ynab-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var dryRun bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync Questrade account balances to YNAB",
	Long:  "Fetch investment account balances from Questrade and update the corresponding accounts in YNAB by creating transactions.",
	Run: func(cmd *cobra.Command, args []string) {
		if err := loadConfig(); err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		// Ensure a valid Questrade client (will refresh or prompt as needed)
		qClient, err := ensureValidQuestradeClient()
		if err != nil {
			fmt.Printf("Error ensuring Questrade auth: %v\n", err)
			os.Exit(1)
		}

		// Ensure YNAB values are present
		ynabToken := viper.GetString("ynab_access_token")
		budgetID := viper.GetString("ynab_budget_id")
		if ynabToken == "" || budgetID == "" {
			fmt.Println("Missing required configuration. Please run 'questrade-ynab auth set' or 'questrade-ynab auth login' first")
			os.Exit(1)
		}

		// Read mapping from ~/.questrade-ynab/mappings.json
		configDir := getConfigDir()
		mappingPath := filepath.Join(configDir, "mappings.json")
		mappingData, err := os.ReadFile(mappingPath)
		if err != nil {
			fmt.Printf("Error reading mappings.json: %v\n", err)
			os.Exit(1)
		}
		var accountMapping map[string]string
		if err := json.Unmarshal(mappingData, &accountMapping); err != nil {
			fmt.Printf("Error parsing mappings.json: %v\n", err)
			os.Exit(1)
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		yClient := ynab.NewClient(ynabToken, ynab.WithUserAgent("questrade-ynab"))

		// Get Questrade accounts
		fmt.Println("Fetching Questrade accounts...")
		qAccounts, err := qClient.GetAccounts()
		if err != nil {
			fmt.Printf("Error fetching Questrade accounts: %v\n", err)
			os.Exit(1)
		}
		if len(qAccounts) == 0 {
			fmt.Println("No Questrade accounts found")
			os.Exit(1)
		}

		// Get YNAB accounts
		fmt.Println("Fetching YNAB accounts...")
		yAccounts, err := yClient.GetAccounts(ctx, budgetID)
		if err != nil {
			fmt.Printf("Error fetching YNAB accounts: %v\n", err)
			os.Exit(1)
		}
		yAccountsMap := make(map[string]*ynab.Account)
		for i := range yAccounts {
			yAccountsMap[yAccounts[i].ID] = &yAccounts[i]
		}

		// Build and show planned transactions
		fmt.Println("\nPreparing transactions...")
		type PlannedTx struct {
			QuestradeName string
			YNABName      string
			YNABAccountID string
			OldBalance    ynab.Milliunits
			NewBalance    ynab.Milliunits
			Amount        ynab.Milliunits
		}
		var planned []PlannedTx
		for qNum, yID := range accountMapping {
			// Find Questrade account
			var qAcc *questrade.Account
			for i := range qAccounts {
				if qAccounts[i].Number == qNum {
					qAcc = &qAccounts[i]
					break
				}
			}
			if qAcc == nil || qAcc.Balances == nil || len(qAcc.Balances.CombinedBalances) == 0 {
				log.Printf("Skipping Questrade account %s: no balance info", qNum)
				continue
			}
			qBalance := qAcc.Balances.CombinedBalances[0].TotalEquity
			yAcc, ok := yAccountsMap[yID]
			if !ok {
				log.Printf("Skipping mapping for Questrade %s: YNAB account %s not found", qNum, yID)
				continue
			}
			oldMilli, newMilli, diff := qysync.PlanAdjustment(yAcc.Balance, qBalance)
			if diff == 0 {
				continue
			}
			planned = append(planned, PlannedTx{
				QuestradeName: fmt.Sprintf("%s (%s)", qAcc.Number, qAcc.Type),
				YNABName:      yAcc.Name,
				YNABAccountID: yAcc.ID,
				OldBalance:    oldMilli,
				NewBalance:    newMilli,
				Amount:        diff,
			})
		}

		if len(planned) == 0 {
			fmt.Println("No transactions needed; all balances match.")
			return
		}

		fmt.Println("Planned transactions:")
		for _, tx := range planned {
			fmt.Printf("  %s → %s: $%.2f → $%.2f (delta: $%.2f)\n", tx.QuestradeName, tx.YNABName, float64(tx.OldBalance)/1000, float64(tx.NewBalance)/1000, float64(tx.Amount)/1000)
		}

		if dryRun {
			fmt.Println("\n[DRY RUN] No transactions created.")
			return
		}

		// Manual approval step
		var response string
		fmt.Print("\nDo you want to create these transactions in YNAB? Type 'yes' to approve: ")
		fmt.Scanln(&response)
		if strings.ToLower(strings.TrimSpace(response)) != "yes" {
			fmt.Println("Aborted: No transactions created.")
			return
		}

		// Actually create transactions
		today := ynab.NewDate(time.Now())
		for _, tx := range planned {
			importID := qysync.AdjustmentImportID(tx.YNABAccountID, today, tx.OldBalance, tx.NewBalance)
			ynabTx := ynab.NewTransaction{
				AccountID: tx.YNABAccountID,
				Date:      today,
				Amount:    tx.Amount,
				PayeeName: "Stock Market",
				Memo:      "Questrade sync",
				Cleared:   ynab.Cleared,
				Approved:  true,
				ImportID:  importID,
			}
			res, err := yClient.CreateTransactions(ctx, budgetID, []ynab.NewTransaction{ynabTx})
			switch {
			case err != nil:
				fmt.Printf("Error creating transaction for %s: %v\n", tx.YNABName, err)
			case qysync.IsDuplicateImportID(res, importID):
				fmt.Printf("= Already in YNAB for %s: $%.2f (duplicate import_id, skipped)\n", tx.YNABName, float64(tx.Amount)/1000)
			default:
				fmt.Printf("✓ Created transaction for %s: $%.2f\n", tx.YNABName, float64(tx.Amount)/1000)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show planned transactions but do not create them")
}
