package cmd

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/brymastr/questrade-ynab/internal/api"
	"github.com/brymastr/questrade-ynab/internal/db"
	"github.com/brymastr/questrade-ynab/internal/scheduler"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web server",
	Long:  "Start the HTTP API server and background sync scheduler.",
	Run: func(cmd *cobra.Command, args []string) {
		dbPath := os.Getenv("DATABASE_PATH")
		if dbPath == "" {
			dbPath = "./data/app.db"
		}

		if err := os.MkdirAll("./data", 0700); err != nil {
			log.Fatalf("create data dir: %v", err)
		}

		store, err := db.Open(dbPath)
		if err != nil {
			log.Fatalf("open db: %v", err)
		}
		defer store.Close()

		sched := scheduler.New(store)
		sched.Start()
		defer sched.Stop()

		router := api.NewRouter(store, sched)

		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		addr := fmt.Sprintf(":%s", port)
		log.Printf("server listening on %s", addr)
		if err := http.ListenAndServe(addr, router); err != nil {
			log.Fatalf("server error: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
