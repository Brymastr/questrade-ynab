package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/brymastr/questrade-ynab/cmd"
	"github.com/brymastr/questrade-ynab/internal/api"
	"github.com/brymastr/questrade-ynab/internal/db"
)

// noopScheduler satisfies api.ScheduleManager for environments without an
// in-process cron (i.e. Lambda). Scheduled sync is handled elsewhere in prod.
type noopScheduler struct{}

func (noopScheduler) Register(userID, cronExpr string) {}
func (noopScheduler) Deregister(userID string)         {}

func main() {
	// In Lambda, serve the same chi router through the API Gateway v2 adapter
	// (Function URL). Locally, fall through to the normal cobra CLI (serve, etc.).
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		runLambda()
		return
	}
	cmd.Execute()
}

func runLambda() {
	ctx := context.Background()
	if err := loadSecrets(ctx); err != nil {
		log.Fatalf("load secrets: %v", err)
	}
	store, err := db.New(ctx)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	router := api.NewRouter(store, noopScheduler{})
	lambda.Start(httpadapter.NewV2(router).ProxyWithContext)
}

// loadSecrets fetches a JSON secret from AWS Secrets Manager (identified by the
// SECRETS_ARN env var) and exports each key into the process environment, so the
// rest of the app reads them via os.Getenv as usual. Existing env vars are not
// overwritten. A missing SECRETS_ARN is a no-op (e.g. local dev via .env).
func loadSecrets(ctx context.Context) error {
	arn := os.Getenv("SECRETS_ARN")
	if arn == "" {
		return nil
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load aws config: %w", err)
	}
	out, err := secretsmanager.NewFromConfig(cfg).GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(arn),
	})
	if err != nil {
		return fmt.Errorf("get secret %s: %w", arn, err)
	}
	if out.SecretString == nil {
		return nil
	}
	var kv map[string]string
	if err := json.Unmarshal([]byte(*out.SecretString), &kv); err != nil {
		return fmt.Errorf("parse secret json: %w", err)
	}
	for k, v := range kv {
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
	return nil
}
