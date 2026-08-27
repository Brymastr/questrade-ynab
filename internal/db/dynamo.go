package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

// DynamoStore is the DynamoDB implementation of Store, used in AWS Lambda.
//
// Single-table design keyed by (PK, SK):
//
//	PK = USER#<id>
//	SK = PROFILE | TOKEN#<provider> | MAPPING#<id> | SCHEDULE | HISTORY#<rfc3339>#<uuid>
type DynamoStore struct {
	client *dynamodb.Client
	table  string
}

// NewDynamoStore builds a DynamoStore using the default AWS config (region and
// credentials from the environment / Lambda execution role).
func NewDynamoStore(ctx context.Context, table string) (*DynamoStore, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return &DynamoStore{client: dynamodb.NewFromConfig(cfg), table: table}, nil
}

func (d *DynamoStore) Close() error { return nil }

// --- key helpers ---

const (
	skProfile  = "PROFILE"
	skSchedule = "SCHEDULE"
)

func userPK(id string) string        { return "USER#" + id }
func skToken(provider string) string { return "TOKEN#" + provider }
func skMapping(id string) string     { return "MAPPING#" + id }
func userIDFromPK(pk string) string  { return strings.TrimPrefix(pk, "USER#") }

// --- attribute helpers ---

func avS(v string) types.AttributeValue  { return &types.AttributeValueMemberS{Value: v} }
func avBool(v bool) types.AttributeValue { return &types.AttributeValueMemberBOOL{Value: v} }
func avN(n int) types.AttributeValue     { return &types.AttributeValueMemberN{Value: strconv.Itoa(n)} }

func getS(item map[string]types.AttributeValue, k string) string {
	if v, ok := item[k].(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

func getBool(item map[string]types.AttributeValue, k string) bool {
	if v, ok := item[k].(*types.AttributeValueMemberBOOL); ok {
		return v.Value
	}
	return false
}

func getN(item map[string]types.AttributeValue, k string) int {
	if v, ok := item[k].(*types.AttributeValueMemberN); ok {
		n, _ := strconv.Atoi(v.Value)
		return n
	}
	return 0
}

func parseTime(v string) time.Time {
	t, _ := time.Parse(time.RFC3339, v)
	return t
}

// --- users ---

func (d *DynamoStore) CreateUser() (string, error) {
	id := uuid.New().String()
	_, err := d.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(d.table),
		Item: map[string]types.AttributeValue{
			"PK":         avS(userPK(id)),
			"SK":         avS(skProfile),
			"created_at": avS(time.Now().UTC().Format(time.RFC3339)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

func (d *DynamoStore) UserExists(id string) (bool, error) {
	out, err := d.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(d.table),
		Key:       map[string]types.AttributeValue{"PK": avS(userPK(id)), "SK": avS(skProfile)},
	})
	if err != nil {
		return false, err
	}
	return out.Item != nil, nil
}

// --- oauth tokens ---

func (d *DynamoStore) UpsertToken(t OAuthToken) error {
	_, err := d.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(d.table),
		Item: map[string]types.AttributeValue{
			"PK":            avS(userPK(t.UserID)),
			"SK":            avS(skToken(t.Provider)),
			"access_token":  avS(t.AccessToken),
			"refresh_token": avS(t.RefreshToken),
			"expires_at":    avS(t.ExpiresAt.UTC().Format(time.RFC3339)),
			"api_server":    avS(t.APIServer),
		},
	})
	return err
}

func (d *DynamoStore) GetToken(userID, provider string) (*OAuthToken, error) {
	out, err := d.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(d.table),
		Key:       map[string]types.AttributeValue{"PK": avS(userPK(userID)), "SK": avS(skToken(provider))},
	})
	if err != nil {
		return nil, err
	}
	if out.Item == nil {
		return nil, nil
	}
	return &OAuthToken{
		UserID:       userID,
		Provider:     provider,
		AccessToken:  getS(out.Item, "access_token"),
		RefreshToken: getS(out.Item, "refresh_token"),
		ExpiresAt:    parseTime(getS(out.Item, "expires_at")),
		APIServer:    getS(out.Item, "api_server"),
	}, nil
}

func (d *DynamoStore) HasToken(userID, provider string) (bool, error) {
	t, err := d.GetToken(userID, provider)
	return t != nil, err
}

// --- mappings ---

func (d *DynamoStore) GetMappings(userID string) ([]Mapping, error) {
	out, err := d.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(d.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": avS(userPK(userID)),
			":sk": avS("MAPPING#"),
		},
	})
	if err != nil {
		return nil, err
	}
	var mappings []Mapping
	for _, item := range out.Items {
		mappings = append(mappings, Mapping{
			ID:                     strings.TrimPrefix(getS(item, "SK"), "MAPPING#"),
			UserID:                 userID,
			QuestradeAccountNumber: getS(item, "questrade_account_number"),
			YNABBudgetID:           getS(item, "ynab_budget_id"),
			YNABAccountID:          getS(item, "ynab_account_id"),
		})
	}
	return mappings, nil
}

// ReplaceMappings deletes the user's mappings and writes the new set. Not fully
// atomic (delete then write), which is acceptable for this single-user workload.
func (d *DynamoStore) ReplaceMappings(userID string, mappings []Mapping) error {
	ctx := context.Background()
	existing, err := d.GetMappings(userID)
	if err != nil {
		return err
	}
	for _, m := range existing {
		if _, err := d.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(d.table),
			Key:       map[string]types.AttributeValue{"PK": avS(userPK(userID)), "SK": avS(skMapping(m.ID))},
		}); err != nil {
			return err
		}
	}
	for _, m := range mappings {
		id := uuid.New().String()
		if _, err := d.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(d.table),
			Item: map[string]types.AttributeValue{
				"PK":                       avS(userPK(userID)),
				"SK":                       avS(skMapping(id)),
				"questrade_account_number": avS(m.QuestradeAccountNumber),
				"ynab_budget_id":           avS(m.YNABBudgetID),
				"ynab_account_id":          avS(m.YNABAccountID),
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (d *DynamoStore) DeleteMapping(userID, mappingID string) error {
	_, err := d.client.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(d.table),
		Key:       map[string]types.AttributeValue{"PK": avS(userPK(userID)), "SK": avS(skMapping(mappingID))},
	})
	return err
}

// --- sync schedule ---

func (d *DynamoStore) GetSchedule(userID string) (*SyncSchedule, error) {
	out, err := d.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(d.table),
		Key:       map[string]types.AttributeValue{"PK": avS(userPK(userID)), "SK": avS(skSchedule)},
	})
	if err != nil {
		return nil, err
	}
	if out.Item == nil {
		return &SyncSchedule{UserID: userID, CronExpression: "0 9 * * *", Enabled: false}, nil
	}
	sc := &SyncSchedule{
		UserID:         userID,
		CronExpression: getS(out.Item, "cron_expression"),
		Enabled:        getBool(out.Item, "enabled"),
	}
	if nr := getS(out.Item, "next_run_at"); nr != "" {
		t := parseTime(nr)
		sc.NextRunAt = &t
	}
	return sc, nil
}

func (d *DynamoStore) UpsertSchedule(sc SyncSchedule) error {
	item := map[string]types.AttributeValue{
		"PK":              avS(userPK(sc.UserID)),
		"SK":              avS(skSchedule),
		"user_id":         avS(sc.UserID),
		"cron_expression": avS(sc.CronExpression),
		"enabled":         avBool(sc.Enabled),
	}
	if sc.NextRunAt != nil {
		item["next_run_at"] = avS(sc.NextRunAt.UTC().Format(time.RFC3339))
	}
	_, err := d.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(d.table),
		Item:      item,
	})
	return err
}

func (d *DynamoStore) GetAllEnabledSchedules() ([]SyncSchedule, error) {
	out, err := d.client.Scan(context.Background(), &dynamodb.ScanInput{
		TableName:        aws.String(d.table),
		FilterExpression: aws.String("SK = :sk AND #en = :true"),
		ExpressionAttributeNames: map[string]string{
			"#en": "enabled",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sk":   avS(skSchedule),
			":true": avBool(true),
		},
	})
	if err != nil {
		return nil, err
	}
	var schedules []SyncSchedule
	for _, item := range out.Items {
		sc := SyncSchedule{
			UserID:         userIDFromPK(getS(item, "PK")),
			CronExpression: getS(item, "cron_expression"),
			Enabled:        getBool(item, "enabled"),
		}
		if nr := getS(item, "next_run_at"); nr != "" {
			t := parseTime(nr)
			sc.NextRunAt = &t
		}
		schedules = append(schedules, sc)
	}
	return schedules, nil
}

// --- sync history ---

func (d *DynamoStore) CreateSyncHistory(h SyncHistory) error {
	ranAt := h.RanAt.UTC().Format(time.RFC3339)
	sk := fmt.Sprintf("HISTORY#%s#%s", ranAt, uuid.New().String())
	_, err := d.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(d.table),
		Item: map[string]types.AttributeValue{
			"PK":              avS(userPK(h.UserID)),
			"SK":              avS(sk),
			"user_id":         avS(h.UserID),
			"ran_at":          avS(ranAt),
			"status":          avS(h.Status),
			"accounts_synced": avN(h.AccountsSynced),
			"detail":          avS(h.Detail),
		},
	})
	return err
}

func (d *DynamoStore) GetSyncHistory(userID string, limit int) ([]SyncHistory, error) {
	out, err := d.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(d.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": avS(userPK(userID)),
			":sk": avS("HISTORY#"),
		},
		ScanIndexForward: aws.Bool(false), // newest first
		Limit:            aws.Int32(int32(limit)),
	})
	if err != nil {
		return nil, err
	}
	var history []SyncHistory
	for _, item := range out.Items {
		history = append(history, SyncHistory{
			ID:             strings.TrimPrefix(getS(item, "SK"), "HISTORY#"),
			UserID:         userID,
			RanAt:          parseTime(getS(item, "ran_at")),
			Status:         getS(item, "status"),
			AccountsSynced: getN(item, "accounts_synced"),
			Detail:         getS(item, "detail"),
		})
	}
	return history, nil
}
