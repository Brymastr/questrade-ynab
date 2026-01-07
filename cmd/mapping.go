package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/brymastr/questrade-ynab/internal/ynab"
	"github.com/manifoldco/promptui"
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

var mappingCmd = &cobra.Command{
	Use:   "mapping",
	Short: "Manage account mappings between Questrade and YNAB",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
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

var mappingSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Interactive account mapping setup (auth must already be configured)",
	Run: func(cmd *cobra.Command, args []string) {
		// Ensure we have a valid Questrade client (will prompt or refresh as needed)
		qClient, err := ensureValidQuestradeClient()
		if err != nil {
			fmt.Printf("Error ensuring Questrade auth: %v\n", err)
			os.Exit(1)
		}

		// Ensure YNAB values are present
		ynabToken := viper.GetString("ynab_access_token")
		budgetID := viper.GetString("ynab_budget_id")
		if ynabToken == "" || budgetID == "" {
			fmt.Println("Missing required YNAB configuration. Please run 'questrade-ynab auth set' first")
			os.Exit(1)
		}

		yClient := ynab.NewClient(ynabToken, budgetID)

		// Get accounts
		fmt.Println("\nFetching accounts for mapping setup...")
		qAccounts, err := qClient.GetAccounts()
		if err != nil {
			fmt.Printf("Error fetching Questrade accounts: %v\n", err)
			os.Exit(1)
		}
		yAccounts, err := yClient.GetAccounts()
		if err != nil {
			fmt.Printf("Error fetching YNAB accounts: %v\n", err)
			os.Exit(1)
		}
		accountMapping := make(map[string]string)
		for {
			// Prepare Questrade account options
			qOptions := []string{}
			for _, acc := range qAccounts {
				balanceStr := "N/A"
				if acc.Balances != nil && len(acc.Balances.CombinedBalances) > 0 {
					balanceStr = fmt.Sprintf("$%.2f", acc.Balances.CombinedBalances[0].TotalEquity)
				}
				mapped := ""
				if ynabID, ok := accountMapping[acc.Number]; ok {
					mapped = fmt.Sprintf(" [MAPPED to %s]", ynabID)
				}
				qOptions = append(qOptions, fmt.Sprintf("Account #%s (%s) - Balance: %s%s", acc.Number, acc.Type, balanceStr, mapped))
			}
			qOptions = append(qOptions, "Finish mapping")

			templates := &promptui.SelectTemplates{
				Label:    "{{ . }}",
				Active:   "\u001b[1;44m> {{ . }}\u001b[0m",
				Inactive: "  {{ . }}",
				Selected: "\u001b[1;32m✔ {{ . }}\u001b[0m",
			}

			prompt := promptui.Select{
				Label:     "Select a Questrade account to map (select 'Finish mapping' to save and exit)",
				Items:     qOptions,
				Size:      10,
				Templates: templates,
			}
			idx, _, err := prompt.Run()
			if err != nil {
				fmt.Printf("Prompt error: %v\n", err)
				break
			}
			if idx == len(qOptions)-1 {
				// User selected 'Finish mapping'
				break
			}
			selectedQAccount := &qAccounts[idx]

			// Prepare YNAB account options
			yOptions := []string{}
			for _, acc := range yAccounts {
				balanceStr := fmt.Sprintf("$%.2f", float64(acc.Balance)/1000)
				yOptions = append(yOptions, fmt.Sprintf("%s (%s) - Balance: %s", acc.Name, acc.Type, balanceStr))
			}
			yTemplates := &promptui.SelectTemplates{
				Label:    "{{ . }}",
				Active:   "\u001b[1;44m> {{ . }}\u001b[0m",
				Inactive: "  {{ . }}",
				Selected: "\u001b[1;32m✔ {{ . }}\u001b[0m",
			}
			yPrompt := promptui.Select{
				Label:     fmt.Sprintf("Map Questrade #%s to YNAB account", selectedQAccount.Number),
				Items:     yOptions,
				Size:      10,
				Templates: yTemplates,
			}
			yIdx, _, err := yPrompt.Run()
			if err != nil {
				fmt.Printf("Prompt error: %v\n", err)
				continue
			}
			selectedYAccount := yAccounts[yIdx]
			accountMapping[selectedQAccount.Number] = selectedYAccount.ID
			fmt.Printf("✓ Mapped Questrade Account #%s to YNAB Account '%s'\n", selectedQAccount.Number, selectedYAccount.Name)
		}

		// Convert mapping to JSON and persist only mapping to viper/yaml
		mappingJSON, err := json.Marshal(accountMapping)
		if err != nil {
			fmt.Printf("Error creating account mapping: %v\n", err)
			os.Exit(1)
		}

		configDir := getConfigDir()
		if err := os.MkdirAll(configDir, 0700); err != nil {
			fmt.Printf("Error creating config directory: %v\n", err)
			os.Exit(1)
		}

		viper.SetDefault("account_mapping", string(mappingJSON))

		// Write mapping to flat JSON file called mappings.json
		mappingPath := filepath.Join(configDir, "mappings.json")
		if err := os.WriteFile(mappingPath, mappingJSON, 0600); err != nil {
			fmt.Printf("Error writing mappings.json file: %v\n", err)
			os.Exit(1)
		}

		// Display summary
		fmt.Println("\n" + strings.Repeat("=", 50))
		fmt.Println("Mapping Summary:")
		fmt.Println(strings.Repeat("=", 50))
		fmt.Printf("Mapping saved to %s\n", mappingPath)
		fmt.Printf("\nAccount Mappings:\n")
		for qAcctNum, yAcctID := range accountMapping {
			var yAcctName string
			for _, acc := range yAccounts {
				if acc.ID == yAcctID {
					yAcctName = acc.Name
					break
				}
			}
			fmt.Printf("  Questrade #%s → YNAB: %s\n", qAcctNum, yAcctName)
		}
		if len(accountMapping) == 0 {
			fmt.Println("  No accounts mapped")
		}
	},
}

func init() {
	mappingCmd.AddCommand(mappingListCmd)
	mappingCmd.AddCommand(mappingSetCmd)
}
