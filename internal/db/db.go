package db

import (
	"context"
	"fmt"
	"os"
)

// Store is the persistence interface used by the API and scheduler. It has two
// implementations: SQLiteStore (local dev) and DynamoStore (AWS Lambda).
type Store interface {
	CreateUser() (string, error)
	UserExists(id string) (bool, error)

	UpsertToken(t OAuthToken) error
	GetToken(userID, provider string) (*OAuthToken, error)
	HasToken(userID, provider string) (bool, error)

	GetMappings(userID string) ([]Mapping, error)
	ReplaceMappings(userID string, mappings []Mapping) error
	DeleteMapping(userID, mappingID string) error

	GetSchedule(userID string) (*SyncSchedule, error)
	UpsertSchedule(sc SyncSchedule) error
	GetAllEnabledSchedules() ([]SyncSchedule, error)

	CreateSyncHistory(h SyncHistory) error
	GetSyncHistory(userID string, limit int) ([]SyncHistory, error)

	Close() error
}

// New selects and opens a Store based on the DB_BACKEND env var:
//   - "dynamo": DynamoDB (uses DYNAMO_TABLE and the default AWS config)
//   - anything else (default): SQLite (uses DATABASE_PATH, default ./data/app.db)
func New(ctx context.Context) (Store, error) {
	switch os.Getenv("DB_BACKEND") {
	case "dynamo":
		table := os.Getenv("DYNAMO_TABLE")
		if table == "" {
			return nil, fmt.Errorf("DYNAMO_TABLE is required when DB_BACKEND=dynamo")
		}
		return NewDynamoStore(ctx, table)
	default:
		path := os.Getenv("DATABASE_PATH")
		if path == "" {
			path = "./data/app.db"
		}
		return OpenSQLite(path)
	}
}
