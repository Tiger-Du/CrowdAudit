package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opensearch-project/opensearch-go/v2"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/indexer"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/infra/secrets"
)

var (
	once    sync.Once
	initErr error

	pg       *pgxpool.Pool
	osClient *opensearch.Client
)

func initOnce() {
	ctx := context.Background()
	var err error

	pgURL, err := loadPGURL(ctx)
	if err != nil {
		initErr = err
		return
	}

	pg, err = pgxpool.New(ctx, pgURL)
	if err != nil {
		initErr = err
		return
	}

	osURL := getenv("OS_URL", "http://localhost:9200")
	user := os.Getenv("OS_USERNAME")
	pass := os.Getenv("OS_PASSWORD")
	insecure := strings.ToLower(os.Getenv("OS_INSECURE")) == "true"
	if pass == "" {
		initErr = errf("OS_PASSWORD is empty")
		return
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}

	osClient, err = opensearch.NewClient(opensearch.Config{
		Addresses: []string{osURL},
		Username:  user,
		Password:  pass,
		Transport: tr,
	})
	if err != nil {
		initErr = err
		return
	}

	// Optional: ensure index exists (you can keep your ensurePairsIndex in internal/indexer too)
	const pairsIndex = "pairs_v1"

	if err := indexer.EnsurePairsIndex(ctx, osClient, pairsIndex); err != nil {
		initErr = err

		return
	}
}

func handler(ctx context.Context, ev events.SQSEvent) (events.SQSEventResponse, error) {
	once.Do(initOnce)
	if initErr != nil {
		// If init fails, fail the whole batch (retry)
		return events.SQSEventResponse{}, initErr
	}

	// Best practice: process each message; if one fails, return error to retry, and rely on DLQ after max receives.
	// To avoid one poison message/error blocking/retrying the (whole) batch.

	failures := make([]events.SQSBatchItemFailure, 0)

	for _, r := range ev.Records {
		if err := indexer.HandleMessage(ctx, pg, osClient, []byte(r.Body)); err != nil {
			log.Printf("handle error msgId=%s err=%v", r.MessageId, err)
			failures = append(failures, events.SQSBatchItemFailure{
				ItemIdentifier: r.MessageId,
			})
		}
	}

	return events.SQSEventResponse{BatchItemFailures: failures}, nil
}

func main() {
	lambda.Start(handler)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func errf(msg string) error { return &simpleErr{msg: msg} }

type simpleErr struct{ msg string }

func (e *simpleErr) Error() string { return e.msg }

func loadPGURL(ctx context.Context) (string, error) {
	if arn := os.Getenv("PG_URL_SECRET_ARN"); arn != "" {
		return secrets.FetchPGURLFromSecretsManager(ctx, arn)
	}
	if url := os.Getenv("PG_URL"); url != "" {
		return url, nil
	}
	return "", errf("PG_URL or PG_URL_SECRET_ARN must be set")
}
