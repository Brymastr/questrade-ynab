package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brymastr/questrade-ynab/internal/questrade"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage stored authentication values",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var authSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Prompt for auth tokens and persist to config.json (replaces file)",
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)

		fmt.Print("Enter your Questrade manual authorization token (refresh token): ")
		questradeRefreshToken, _ := reader.ReadString('\n')
		questradeRefreshToken = strings.TrimSpace(questradeRefreshToken)

		fmt.Print("Enter your YNAB personal access token: ")
		ynabToken, _ := reader.ReadString('\n')
		ynabToken = strings.TrimSpace(ynabToken)

		fmt.Print("Enter your YNAB budget ID: ")
		budgetID, _ := reader.ReadString('\n')
		budgetID = strings.TrimSpace(budgetID)

		// Ensure config directory exists
		configDir := getConfigDir()
		if err := os.MkdirAll(configDir, 0700); err != nil {
			fmt.Printf("Error creating config directory: %v\n", err)
			os.Exit(1)
		}

		cfg := map[string]interface{}{
			"questrade_refresh_token": questradeRefreshToken,
			"ynab_access_token":       ynabToken,
			"ynab_budget_id":          budgetID,
		}

		b, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			fmt.Printf("Error encoding config JSON: %v\n", err)
			os.Exit(1)
		}

		jsonPath := filepath.Join(configDir, "config.json")
		if err := os.WriteFile(jsonPath, b, 0600); err != nil {
			fmt.Printf("Error writing config.json: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Saved auth values to %s (file replaced)\n", jsonPath)
	},
}

var authShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the current config.json contents",
	Run: func(cmd *cobra.Command, args []string) {
		configDir := getConfigDir()
		jsonPath := filepath.Join(configDir, "config.json")
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("No %s found\n", jsonPath)
				return
			}
			fmt.Printf("Error reading %s: %v\n", jsonPath, err)
			os.Exit(1)
		}

		// Pretty print the JSON (it's already pretty-printed when written, but ensure valid)
		var obj interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			// If invalid JSON, just print raw
			fmt.Println(string(data))
			return
		}
		pretty, _ := json.MarshalIndent(obj, "", "  ")
		fmt.Println(string(pretty))
	},
}

func init() {
	authCmd.AddCommand(authSetCmd)
	authCmd.AddCommand(authShowCmd)
	authCmd.AddCommand(authLoginCmd)
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Ensure Questrade access token is valid; refresh or prompt for new refresh token if needed",
	Run: func(cmd *cobra.Command, args []string) {
		configDir := getConfigDir()
		jsonPath := filepath.Join(configDir, "config.json")

		// Load existing config.json if present
		var m map[string]interface{}
		if data, err := os.ReadFile(jsonPath); err == nil {
			_ = json.Unmarshal(data, &m)
		} else {
			m = make(map[string]interface{})
		}

		// Helper to write config.json
		writeConfig := func() error {
			b, err := json.MarshalIndent(m, "", "  ")
			if err != nil {
				return err
			}
			return os.WriteFile(jsonPath, b, 0600)
		}

		// Get tokens from config map
		refreshToken, _ := m["questrade_refresh_token"].(string)
		accessToken, _ := m["questrade_access_token"].(string)
		apiServer, _ := m["questrade_api_server"].(string)
		expiresIn := 0
		if v, ok := m["questrade_expires_in"]; ok {
			switch t := v.(type) {
			case float64:
				expiresIn = int(t)
			case int:
				expiresIn = t
			}
		}

		// If no refresh token, prompt user to enter one
		reader := bufio.NewReader(os.Stdin)
		if refreshToken == "" {
			fmt.Print("Enter your Questrade manual authorization token (refresh token): ")
			rt, _ := reader.ReadString('\n')
			refreshToken = strings.TrimSpace(rt)
			m["questrade_refresh_token"] = refreshToken
			if err := writeConfig(); err != nil {
				fmt.Printf("Warning: failed to write config.json: %v\n", err)
			}
		}

		qClient := questrade.NewClient(refreshToken)
		if accessToken != "" && apiServer != "" && expiresIn > 0 {
			qClient.SetAccessToken(accessToken, apiServer, expiresIn)
		}

		// Perform a live validation of the cached access token
		if accessToken != "" && apiServer != "" && expiresIn > 0 {
			if valid, err := qClient.IsAccessTokenValid(accessToken, apiServer); err == nil && valid {
				fmt.Println("Questrade access token is valid; no action needed")
				return
			}
			// Otherwise attempt refresh below
		}

		// Try to refresh
		tr, err := qClient.Refresh()
		if err == nil {
			// Persist returned tokens
			if err := updateConfigJSON(configDir, tr.RefreshToken, tr.AccessToken, tr.APIServer, tr.ExpiresIn); err != nil {
				fmt.Printf("Warning: failed to persist refreshed token: %v\n", err)
			} else {
				fmt.Println("Successfully refreshed access token and updated config.json")
			}
			return
		}

		// If refresh failed, prompt for a new refresh token
		fmt.Printf("Refresh failed: %v\n", err)
		fmt.Print("Enter a new Questrade refresh token: ")
		rt, _ := reader.ReadString('\n')
		rt = strings.TrimSpace(rt)
		if rt == "" {
			fmt.Println("No refresh token provided; aborting")
			os.Exit(1)
		}
		m["questrade_refresh_token"] = rt
		if err := writeConfig(); err != nil {
			fmt.Printf("Warning: failed to write config.json: %v\n", err)
		}

		// Try refresh again with new token
		qClient = questrade.NewClient(rt)
		tr2, err := qClient.Refresh()
		if err != nil {
			fmt.Printf("Failed to refresh with provided token: %v\n", err)
			os.Exit(1)
		}
		if err := updateConfigJSON(configDir, tr2.RefreshToken, tr2.AccessToken, tr2.APIServer, tr2.ExpiresIn); err != nil {
			fmt.Printf("Warning: failed to persist refreshed token: %v\n", err)
		} else {
			fmt.Println("Successfully refreshed access token and updated config.json")
		}
	},
}
