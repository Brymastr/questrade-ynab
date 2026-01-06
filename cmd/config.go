package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brymastr/questrade-ynab/internal/questrade"
	"github.com/brymastr/questrade-ynab/internal/ynab"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var mappingCmd = &cobra.Command{
	Use:   "mapping",
	Short: "Manage account mappings between Questrade and YNAB",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
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

func getConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".questrade-ynab")
}

// updateConfigJSON updates the config.json file with new token values from the client
func updateConfigJSON(configDir string, refreshToken, accessToken, apiServer string, expiresIn int) error {
	jsonPath := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(jsonPath)
	var m map[string]interface{}
	if err == nil {
		_ = json.Unmarshal(data, &m)
	} else {
		m = make(map[string]interface{})
	}

	// Update tokens and expiration
	if refreshToken != "" {
		m["questrade_refresh_token"] = refreshToken
	}
	if accessToken != "" {
		m["questrade_access_token"] = accessToken
	}
	if apiServer != "" {
		m["questrade_api_server"] = apiServer
	}
	if expiresIn > 0 {
		m["questrade_expires_in"] = expiresIn
	}

	jsonBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	return os.WriteFile(jsonPath, jsonBytes, 0600)
}

func loadConfig() error {
	configDir := getConfigDir()
	// If a JSON config exists, prefer it and load values from there (useful for testing)
	jsonPath := filepath.Join(configDir, "config.json")
	if _, err := os.Stat(jsonPath); err == nil {
		data, err := os.ReadFile(jsonPath)
		if err == nil {
			var m map[string]interface{}
			if err := json.Unmarshal(data, &m); err == nil {
				// Inform the user
				fmt.Printf("Reading configuration from %s. Delete this file to change values interactively.\n", jsonPath)
				if v, ok := m["questrade_refresh_token"].(string); ok {
					viper.Set("questrade_refresh_token", v)
				}
				if v, ok := m["questrade_api_server"].(string); ok {
					viper.Set("questrade_api_server", v)
				}
				if v, ok := m["ynab_access_token"].(string); ok {
					viper.Set("ynab_access_token", v)
				}
				if v, ok := m["ynab_budget_id"].(string); ok {
					viper.Set("ynab_budget_id", v)
				}
				// Load cached access token if present
				if v, ok := m["questrade_access_token"].(string); ok {
					viper.Set("questrade_access_token", v)
				}
				// Load cached expiration if present
				if v, ok := m["questrade_expires_in"].(float64); ok {
					viper.Set("questrade_expires_in", int(v))
				}
				// account_mapping may be a map; convert to JSON string expected by rest of app
				if am, ok := m["account_mapping"]; ok {
					switch t := am.(type) {
					case string:
						viper.Set("account_mapping", t)
					default:
						if b, err := json.Marshal(t); err == nil {
							viper.Set("account_mapping", string(b))
						}
					}
				}
				return nil
			}
		}
	}
	viper.AddConfigPath(configDir)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return fmt.Errorf("config file not found. Please run 'questrade-ynab config set' first")
		}
		return err
	}
	return nil
}

// ensureValidQuestradeClient ensures we have a Questrade client with a valid access token.
// It will attempt to validate a cached access token, refresh it if invalid, and prompt the
// user for a new refresh token if refresh fails. The returned client will have a valid
// access token and the config.json will be updated with any rotated tokens.
func ensureValidQuestradeClient() (*questrade.Client, error) {
	// Ensure viper is loaded
	if err := loadConfig(); err != nil {
		return nil, err
	}

	configDir := getConfigDir()
	refreshToken := viper.GetString("questrade_refresh_token")
	accessToken := viper.GetString("questrade_access_token")
	apiServer := viper.GetString("questrade_api_server")
	expiresIn := viper.GetInt("questrade_expires_in")

	// If there's no refresh token, prompt now
	if refreshToken == "" {
		fmt.Print("Enter your Questrade manual authorization token (refresh token): ")
		var rt string
		fmt.Scanln(&rt)
		refreshToken = strings.TrimSpace(rt)
		if refreshToken == "" {
			return nil, fmt.Errorf("no refresh token provided")
		}
		// Persist refresh token to config.json
		if err := updateConfigJSON(configDir, refreshToken, "", "", 0); err != nil {
			// warn but continue
			fmt.Printf("Warning: failed to persist refresh token to config.json: %v\n", err)
		}
	}

	qClient := questrade.NewClient(refreshToken)

	// If we have a cached access token, perform live validation
	if accessToken != "" && apiServer != "" && expiresIn > 0 {
		qClient.SetAccessToken(accessToken, apiServer, expiresIn)
		if valid, err := qClient.IsAccessTokenValid(accessToken, apiServer); err == nil && valid {
			return qClient, nil
		}
	}

	// Try to refresh
	tr, err := qClient.Refresh()
	if err == nil {
		// Persist returned tokens
		if perr := updateConfigJSON(configDir, tr.RefreshToken, tr.AccessToken, tr.APIServer, tr.ExpiresIn); perr != nil {
			fmt.Printf("Warning: failed to persist refreshed token: %v\n", perr)
		}
		return qClient, nil
	}

	// Refresh failed; prompt user for a new refresh token
	fmt.Printf("Refresh failed: %v\n", err)
	fmt.Print("Enter a new Questrade refresh token: ")
	var rt string
	fmt.Scanln(&rt)
	rt = strings.TrimSpace(rt)
	if rt == "" {
		return nil, fmt.Errorf("no refresh token provided")
	}

	// Persist the new refresh token and try again
	if err := updateConfigJSON(configDir, rt, "", "", 0); err != nil {
		fmt.Printf("Warning: failed to persist new refresh token: %v\n", err)
	}

	qClient = questrade.NewClient(rt)
	tr2, err := qClient.Refresh()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh with provided token: %w", err)
	}
	if perr := updateConfigJSON(configDir, tr2.RefreshToken, tr2.AccessToken, tr2.APIServer, tr2.ExpiresIn); perr != nil {
		fmt.Printf("Warning: failed to persist refreshed token: %v\n", perr)
	}
	return qClient, nil
}

func init() {
	mappingCmd.AddCommand(mappingSetCmd)
	rootCmd.AddCommand(mappingCmd)
	mappingCmd.AddCommand(mappingSetCmd)
}
