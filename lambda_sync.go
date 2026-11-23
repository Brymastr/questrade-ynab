package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/brymastr/questrade-ynab/internal/db"
	"github.com/brymastr/questrade-ynab/internal/scheduler"
)

// Scheduled sync on Lambda: EventBridge Scheduler ticks every tickInterval,
// the tick enqueues one SQS message per user whose slot fell in the interval,
// and the queue's event source mapping (with a concurrency cap) runs them. Users'
// slots are spread over the evening, and the cap bounds load on Questrade and
// YNAB even if many users share a slot.

// schedulerTickSource marks the payload EventBridge Scheduler sends (set in
// the CDK stack). Function URL requests can't produce it: their payload is the
// API Gateway v2 envelope, whose top-level keys the caller doesn't control.
const schedulerTickSource = "questrade-ynab.scheduler"

// tickInterval must match the EventBridge schedule in the CDK stack.
const tickInterval = 5 * time.Minute

type schedulerTick struct {
	Source string `json:"source"`
	// ScheduledTime is the slot this invocation is for, filled in by
	// EventBridge Scheduler (<aws.scheduler.scheduled-time>). Using it rather
	// than the wall clock keeps windows contiguous when a tick runs late.
	ScheduledTime string `json:"scheduled_time"`
}

type syncJob struct {
	UserID string `json:"user_id"`
}

type syncQueue struct {
	client *sqs.Client
	url    string
}

func newSyncQueue(ctx context.Context) (*syncQueue, error) {
	url := os.Getenv("SYNC_QUEUE_URL")
	if url == "" {
		return nil, fmt.Errorf("SYNC_QUEUE_URL is not set")
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &syncQueue{client: sqs.NewFromConfig(cfg), url: url}, nil
}

func (q *syncQueue) enqueueDue(ctx context.Context, store db.Store, tick schedulerTick) error {
	to, err := time.Parse(time.RFC3339, tick.ScheduledTime)
	if err != nil {
		to = time.Now().Truncate(tickInterval)
	}
	users, err := scheduler.DueUsers(store, to.Add(-tickInterval), to)
	if err != nil {
		return err
	}
	// SendMessageBatch takes at most 10 entries.
	for start := 0; start < len(users); start += 10 {
		end := min(start+10, len(users))
		entries := make([]sqstypes.SendMessageBatchRequestEntry, 0, end-start)
		for i, u := range users[start:end] {
			body, _ := json.Marshal(syncJob{UserID: u})
			entries = append(entries, sqstypes.SendMessageBatchRequestEntry{
				Id:          aws.String(strconv.Itoa(i)),
				MessageBody: aws.String(string(body)),
			})
		}
		out, err := q.client.SendMessageBatch(ctx, &sqs.SendMessageBatchInput{
			QueueUrl: aws.String(q.url),
			Entries:  entries,
		})
		if err != nil {
			return fmt.Errorf("enqueue sync jobs: %w", err)
		}
		for _, f := range out.Failed {
			log.Printf("scheduler: enqueue failed for entry %s: %s", aws.ToString(f.Id), aws.ToString(f.Message))
		}
	}
	return nil
}

// runSyncJobs runs each queued user's sync. Sync failures (expired tokens and
// the like) are recorded in the user's ledger rather than retried, since a
// retry wouldn't fix them; only malformed messages are reported as failures.
func runSyncJobs(store db.Store, ev events.SQSEvent) events.SQSEventResponse {
	var resp events.SQSEventResponse
	for _, rec := range ev.Records {
		var job syncJob
		if err := json.Unmarshal([]byte(rec.Body), &job); err != nil || job.UserID == "" {
			log.Printf("scheduler: bad sync job %s: %q", rec.MessageId, rec.Body)
			resp.BatchItemFailures = append(resp.BatchItemFailures, events.SQSBatchItemFailure{ItemIdentifier: rec.MessageId})
			continue
		}
		scheduler.RunForUser(store, job.UserID)
	}
	return resp
}
