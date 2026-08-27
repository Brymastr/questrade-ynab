package cmd

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "questrade-ynab",
	Short: "Sync Questrade investment accounts with YNAB",
	Long: `A CLI application that fetches current investment account values from Questrade
and updates the corresponding accounts in YNAB (You Need A Budget).`,
	// Load .env before any command runs so OAuth client IDs/secrets and other
	// config are present in the process environment. Best-effort: in production
	// (e.g. Docker) these come from the real environment and .env won't exist.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		_ = godotenv.Load()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(mappingCmd)
}
