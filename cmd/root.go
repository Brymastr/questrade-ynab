package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "questrade-ynab",
	Short: "Sync Questrade investment accounts with YNAB",
	Long: `A CLI application that fetches current investment account values from Questrade
and updates the corresponding accounts in YNAB (You Need A Budget).`,
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
}
