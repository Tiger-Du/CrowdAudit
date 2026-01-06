package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/segmentio/kafka-go"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/indexer"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// graceful shutdown
	go func() {
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		cancel()
	}()

	pgURL := getenv("PG_URL", "postgres://crowdaudit:crowdaudit@localhost:5432/crowdaudit?sslmode=disable")
	pg, err := pgxpool.New(ctx, pgURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pg.Close()

	osURL := getenv("OS_URL", "http://localhost:9200")
	user := os.Getenv("OS_USERNAME")
	pass := os.Getenv("OS_PASSWORD")
	insecure := strings.ToLower(os.Getenv("OS_INSECURE")) == "true"

	if pass == "" {
		log.Fatal("OS_PASSWORD is empty")
	}
	log.Printf("opensearch: url=%s user=%q insecure=%v", osURL, user, insecure)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure, // dev only
		},
	}

	osClient, err := opensearch.NewClient(opensearch.Config{
		Addresses: []string{osURL},
		Username:  user,
		Password:  pass,
		Transport: tr,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Ensure pairs_v1 exists
	if err := indexer.EnsurePairsIndex(ctx, osClient, "pairs_v1"); err != nil {
		log.Fatal(err)
	}

	brokers := strings.Split(getenv("KAFKA_BROKERS", "localhost:9092"), ",")
	groupID := getenv("KAFKA_GROUP_ID", "search-indexer")
	topic := getenv("KAFKA_TOPIC", "search-index")

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		GroupID:     groupID,
		Topic:       topic,
		MinBytes:    1e3,
		MaxBytes:    10e6,
		MaxWait:     500 * time.Millisecond,
		StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()

	log.Printf("indexer up: topic=%s group=%s brokers=%v opensearch=%s", topic, groupID, brokers, osURL)

	for {
		m, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("read message error: %v", err)
			continue
		}

		// Process with timeout; if it fails, we log and continue.
		// Kafka will redeliver if you crash; this is at-least-once.
		if err := indexer.HandleMessage(ctx, pg, osClient, m.Value); err != nil {
			log.Printf("handle error key=%s err=%v", string(m.Key), err)
			// In a "strict" setup you'd return error and stop committing offsets,
			// or route to DLQ. For MVP, logging is okay if you have a repair job.
		}
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
