package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	_ "time/tzdata" // Lambda's base image has no zoneinfo; schedules use Eastern time

	"github.com/aws/aws-lambda-go/events"
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
// in-process cron (i.e. Lambda). There, EventBridge Scheduler ticks read due
// schedules from the store (see lambda_sync.go).
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
	originVerify, err := loadOriginVerify(ctx)
	if err != nil {
		log.Fatalf("load origin verify secret: %v", err)
	}
	store, err := db.New(ctx)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	proxy := httpadapter.NewV2(api.NewRouter(store, noopScheduler{}))
	jobs, err := newSyncQueue(ctx)
	if err != nil {
		log.Fatalf("sync queue: %v", err)
	}

	// One function serves three event shapes: EventBridge Scheduler ticks, SQS
	// sync jobs, and Function URL HTTP requests (via CloudFront).
	lambda.Start(func(ctx context.Context, raw json.RawMessage) (any, error) {
		var tick schedulerTick
		if err := json.Unmarshal(raw, &tick); err == nil && tick.Source == schedulerTickSource {
			return nil, jobs.enqueueDue(ctx, store, tick)
		}
		var sqsEvent events.SQSEvent
		if err := json.Unmarshal(raw, &sqsEvent); err == nil && len(sqsEvent.Records) > 0 &&
			sqsEvent.Records[0].EventSource == "aws:sqs" {
			return runSyncJobs(store, sqsEvent), nil
		}
		var req events.APIGatewayV2HTTPRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("unrecognized event: %w", err)
		}
		// Only CloudFront knows the origin secret; refuse direct hits on the
		// public Function URL. (Function URLs lowercase header names.)
		if originVerify != "" &&
			subtle.ConstantTimeCompare([]byte(req.Headers["x-origin-verify"]), []byte(originVerify)) != 1 {
			return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusForbidden, Body: "Forbidden"}, nil
		}
		return proxy.ProxyWithContext(ctx, req)
	})
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

// loadOriginVerify fetches the plain-string secret CloudFront sends in the
// x-origin-verify header (ORIGIN_VERIFY_SECRET_ARN). Unset means no check, as
// in local development.
func loadOriginVerify(ctx context.Context) (string, error) {
	arn := os.Getenv("ORIGIN_VERIFY_SECRET_ARN")
	if arn == "" {
		return "", nil
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("load aws config: %w", err)
	}
	out, err := secretsmanager.NewFromConfig(cfg).GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(arn),
	})
	if err != nil {
		return "", fmt.Errorf("get secret %s: %w", arn, err)
	}
	v := aws.ToString(out.SecretString)
	if v == "" {
		return "", fmt.Errorf("secret %s is empty", arn)
	}
	return v, nil
}
