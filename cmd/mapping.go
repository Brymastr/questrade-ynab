package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/brymastr/questrade-ynab/internal/ynab"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type UpdatePreview struct {
	QuestradeName   string
	QuestradeName2  string
	QNumber         string
	CurrentBalance  float64
	NewBalance      float64
	YNABAccountID   string
	YNABAccountName string
}

var dryRun bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync Questrade account balances to YNAB",
	Long:  "Fetch investment account balances from Questrade and update the corresponding accounts in YNAB",
	Run: func(cmd *cobra.Command, args []string) {
		if err := loadConfig(); err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		// remember original refresh token so we can persist rotated value to YAML if it changed
		origRefresh := viper.GetString("questrade_refresh_token")

		// Ensure a valid Questrade client (will refresh or prompt as needed)

		qClient, err := ensureValidQuestradeClient()
		if err != nil {
			fmt.Printf("Error ensuring Questrade auth: %v\n", err)
			os.Exit(1)
		}

		// Persist rotated refresh token if it changed
		newRefresh := qClient.GetRefreshToken()
		if newRefresh != "" && newRefresh != origRefresh {
			viper.Set("questrade_refresh_token", newRefresh)
			configDir := getConfigDir()
			if err := viper.WriteConfigAs(filepath.Join(configDir, "config.json")); err != nil {
				log.Printf("Warning: failed to persist refreshed token: %v", err)
			}
		}

		// Ensure YNAB values are present
		ynabToken := viper.GetString("ynab_access_token")
		budgetID := viper.GetString("ynab_budget_id")
		accountMappingStr := viper.GetString("account_mapping")

		if ynabToken == "" || budgetID == "" {
			fmt.Println("Missing required configuration. Please run 'questrade-ynab auth set' or 'questrade-ynab auth login' first")
			os.Exit(1)
		}

		// Parse account mapping
		var accountMapping map[string]string
		if err := json.Unmarshal([]byte(accountMappingStr), &accountMapping); err != nil {
			fmt.Printf("Error parsing account mapping: %v\n", err)
			os.Exit(1)
		}

		yClient := ynab.NewClient(ynabToken, budgetID)
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
		yAccounts, err := yClient.GetAccounts()
		if err != nil {
			fmt.Printf("Error fetching YNAB accounts: %v\n", err)
			os.Exit(1)
		}

		// Create YNAB account lookup by ID
		yAccountsMap := make(map[string]*ynab.Account)
		for i := range yAccounts {
			yAccountsMap[yAccounts[i].ID] = &yAccounts[i]
		}

		// Build preview of changes
		fmt.Println("\nPreparing updates...")
		var updates []UpdatePreview
		skipped := 0

		for _, qAccount := range qAccounts {
			// Check if this account should be synced
			ynabID, ok := accountMapping[qAccount.Number]
			if !ok {
				log.Printf("Skipping Questrade account %s (%s) - not in account mapping\n", qAccount.Number, qAccount.Type)
				skipped++
				continue
			}

			// Get YNAB account
			yAccount, exists := yAccountsMap[ynabID]
			if !exists {
				log.Printf("Skipping Questrade account %s - YNAB account %s not found\n", qAccount.Number, ynabID)
				skipped++
				continue
			}

			// Get current balance
			balance, err := qClient.GetAccountBalances(qAccount.Number)
			if err != nil {
				log.Printf("Error getting balance for account %s: %v\n", qAccount.Number, err)
				skipped++
				continue
			}

			updates = append(updates, UpdatePreview{
				QuestradeName:   qAccount.Number,
				QuestradeName2:  qAccount.Type,
				QNumber:         qAccount.Number,
				CurrentBalance:  float64(yAccount.Balance) / 1000,
				NewBalance:      balance.Total,
				YNABAccountID:   ynabID,
				YNABAccountName: yAccount.Name,
			})
		}

		// Display preview
		fmt.Println("\n" + strings.Repeat("=", 100))
		fmt.Println("SYNC PREVIEW - The following accounts will be updated:")
		fmt.Println(strings.Repeat("=", 100))

		if len(updates) == 0 {
			fmt.Println("No accounts to sync")
			return
		}

		for i, update := range updates {
			fmt.Printf("\n%d. Questrade Account #%s (%s)\n", i+1, update.QNumber, update.QuestradeName2)
			fmt.Printf("   → YNAB Account: %s\n", update.YNABAccountName)
			fmt.Printf("   Current Balance: $%.2f\n", update.CurrentBalance)
			fmt.Printf("   New Balance:     $%.2f\n", update.NewBalance)
			if update.CurrentBalance != update.NewBalance {
				change := update.NewBalance - update.CurrentBalance
				if change > 0 {
					fmt.Printf("   Change:          +$%.2f\n", change)
				} else {
					fmt.Printf("   Change:          -$%.2f\n", -change)
				}
			}
		}

		fmt.Println("\n" + strings.Repeat("=", 100))
		fmt.Printf("Total accounts to update: %d\n", len(updates))
		fmt.Printf("Skipped accounts: %d\n", skipped)

		// Check for dry run
		if dryRun {
			fmt.Println("\n[DRY RUN] No changes were made")
			return
		}

		// Ask for approval
		fmt.Println("\n" + strings.Repeat("=", 100))
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Do you want to proceed with these updates? (yes/no): ")
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "yes" && response != "y" {
			fmt.Println("Sync cancelled")
			return
		}

		// Apply updates
		fmt.Println("\nApplying updates...")
		synced := 0
		failed := 0

		for i, update := range updates {
			// Convert balance to milliunits (1000 = $1)
			balanceMilliunits := int64(update.NewBalance * 1000)

			// Update YNAB account
			if err := yClient.UpdateAccountBalance(update.YNABAccountID, balanceMilliunits); err != nil {
				log.Printf("Error updating YNAB account %s: %v\n", update.YNABAccountID, err)
				failed++
				continue
			}

			fmt.Printf("✓ (%d/%d) Updated %s - New balance: $%.2f\n", i+1, len(updates), update.YNABAccountName, update.NewBalance)
			synced++
		}

		fmt.Println("\n" + strings.Repeat("=", 100))
		fmt.Printf("Sync completed: %d accounts updated, %d failed, %d skipped\n", synced, failed, skipped)
	},
}

var mappingListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Questrade accounts and YNAB accounts for mapping",
	Run: func(cmd *cobra.Command, args []string) {
		if err := loadConfig(); err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		origRefresh := viper.GetString("questrade_refresh_token")

		qClient, err := ensureValidQuestradeClient()
		if err != nil {
			fmt.Printf("Error ensuring Questrade auth: %v\n", err)
			os.Exit(1)
		}

		ynabToken := viper.GetString("ynab_access_token")
		budgetID := viper.GetString("ynab_budget_id")
		if ynabToken == "" || budgetID == "" {
			fmt.Println("Missing required configuration. Please run 'questrade-ynab auth set' or 'questrade-ynab auth login' first")
			os.Exit(1)
		}

		newRefresh := qClient.GetRefreshToken()
		if newRefresh != "" && newRefresh != origRefresh {
			viper.Set("questrade_refresh_token", newRefresh)
			configDir := getConfigDir()
			if err := viper.WriteConfigAs(filepath.Join(configDir, "config.json")); err != nil {
				log.Printf("Warning: failed to persist refreshed token: %v", err)
			}
		}

		yClient := ynab.NewClient(ynabToken, budgetID)

		// Get Questrade accounts (with balances fetched in parallel)
		fmt.Println("\nQuestrade Accounts:")
		fmt.Println("===================")
		qAccounts, err := qClient.GetAccounts()
		if err != nil {
			fmt.Printf("Error fetching Questrade accounts: %v\n", err)
			os.Exit(1)
		}

		// Write Questrade accounts to JSON file for lookup
		configDir := getConfigDir()
		qFile := filepath.Join(configDir, "questrade_accounts.json")
		qData, _ := json.MarshalIndent(qAccounts, "", "  ")
		_ = os.WriteFile(qFile, qData, 0644)

		// Print Questrade accounts (name and balance)
		for _, acc := range qAccounts {
			balanceStr := "N/A"
			if acc.Balances != nil && len(acc.Balances.CombinedBalances) > 0 {
				totalEquity := acc.Balances.CombinedBalances[0].TotalEquity
				balanceStr = fmt.Sprintf("$%.2f", totalEquity)
			}
			fmt.Printf("  %s %s\n", acc.Type, balanceStr)
		}

		// Get YNAB accounts
		fmt.Println("\nYNAB Accounts:")
		fmt.Println("==============")
		yAccounts, err := yClient.GetAccounts()
		if err != nil {
			fmt.Printf("Error fetching YNAB accounts: %v\n", err)
			os.Exit(1)
		}

		// Write YNAB accounts to JSON file for lookup
		yFile := filepath.Join(configDir, "ynab_accounts.json")
		yData, _ := json.MarshalIndent(yAccounts, "", "  ")
		_ = os.WriteFile(yFile, yData, 0644)

		for _, acc := range yAccounts {
			fmt.Printf("  %s $%.2f\n", acc.Name, float64(acc.Balance)/1000)
		}

		// Print mapping of Questrade accounts to YNAB accounts (by name)
		mappingPath := filepath.Join(configDir, "mappings.json")
		var accountMapping map[string]string
		mappingData, err := os.ReadFile(mappingPath)
		if err == nil {
			_ = json.Unmarshal(mappingData, &accountMapping)
		} else {
			accountMapping = make(map[string]string)
		}

		// Build lookup maps for names
		qNumToName := make(map[string]string)
		for _, acc := range qAccounts {
			qNumToName[acc.Number] = acc.Type
		}
		yIDToName := make(map[string]string)
		for _, acc := range yAccounts {
			yIDToName[acc.ID] = acc.Name
		}

		fmt.Println("\nAccount Mappings:")
		fmt.Println("=================")
		if len(accountMapping) == 0 {
			fmt.Println("  No account mappings found.")
		} else {
			for qID, yID := range accountMapping {
				qName := qNumToName[qID]
				yName := yIDToName[yID]
				fmt.Printf("  %s → %s\n", qName, yName)
			}
		}

		// Print mapping helper
		fmt.Println("\nTo create account mappings, use:")
		fmt.Println("  questrade-ynab mapping set")
	},
}

func init() {
	mappingCmd.AddCommand(mappingListCmd)
	syncCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without applying them")
}
