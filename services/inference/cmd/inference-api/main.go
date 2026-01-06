package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	opensearch "github.com/opensearch-project/opensearch-go/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/api"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/dispatcher"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/infra/dburl"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/infra/redisx"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/obs"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/outbox"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/providers"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/ratelimit"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/search"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/voting"
)

// Config holds all environment-based configuration
type Config struct {
	DatabaseURL   string
	RedisURL      string
	EnableRedis   bool
	EnableOutbox  bool
	KafkaBrokers  []string
	EnableSearch  bool
	OpenSearchURL string
	OSUser        string
	OSPass        string
	OSInsecure    bool
	Port          string
	QueueSize     int
	WorkerCount   int
}

func loadConfig(ctx context.Context) (Config, error) {
	dbURL, err := dburl.Load(ctx)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		DatabaseURL:   dbURL,
		RedisURL:      os.Getenv("REDIS_URL"),
		EnableRedis:   os.Getenv("ENABLE_REDIS") != "false",
		EnableOutbox:  os.Getenv("ENABLE_OUTBOX_PUBLISHER") != "false",
		KafkaBrokers:  strings.Split(os.Getenv("KAFKA_BROKERS"), ","),
		EnableSearch:  os.Getenv("ENABLE_SEARCH") != "false",
		OpenSearchURL: getenv("OS_URL", "https://localhost:9200"),
		OSUser:        os.Getenv("OS_USERNAME"),
		OSPass:        os.Getenv("OS_PASSWORD"),
		OSInsecure:    strings.ToLower(os.Getenv("OS_INSECURE")) == "true",
		Port:          ":8080",
		QueueSize:     200,
		WorkerCount:   32,
	}

	// --- Validation Logic ---
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}

	if cfg.EnableRedis && cfg.RedisURL == "" {
		return cfg, fmt.Errorf("REDIS_URL is required when ENABLE_REDIS is true")
	}

	if cfg.EnableOutbox && (len(cfg.KafkaBrokers) == 0 || cfg.KafkaBrokers[0] == "") {
		return cfg, fmt.Errorf("KAFKA_BROKERS is required when ENABLE_OUTBOX_PUBLISHER is true")
	}

	if cfg.EnableSearch && cfg.OSPass == "" {
		return cfg, fmt.Errorf("OS_PASSWORD is required when ENABLE_SEARCH is true")
	}

	return cfg, nil
}

func main() {
	_ = godotenv.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := loadConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}

	obs.MustRegister(prometheus.DefaultRegisterer)

	// --- DB (Postgres) ---

	dbpool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer dbpool.Close()

	// Optional: tune pool (these are conservative defaults)
	dbpool.Config().MaxConns = 20
	dbpool.Config().MinConns = 2
	dbpool.Config().MaxConnIdleTime = 5 * time.Minute
	dbpool.Config().MaxConnLifetime = 30 * time.Minute

	// --- Redis ---

	var rdb *redis.Client
	if cfg.EnableRedis {
		var err error
		rdb, err = redisx.NewClientFromURL(cfg.RedisURL)
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			if err := rdb.Close(); err != nil {
				log.Printf("redis close error: %v", err)
			}
		}()
	}

	// --- Kafka ---

	if cfg.EnableOutbox {
		brokersEnv := os.Getenv("KAFKA_BROKERS")
		if brokersEnv == "" {
			log.Fatal("KAFKA_BROKERS is required when ENABLE_OUTBOX_PUBLISHER != false")
		}
		brokers := strings.Split(brokersEnv, ",")

		writer := outbox.NewWriter(brokers)

		publisher := &outbox.Publisher{
			DB:     dbpool,
			Writer: writer,
			// BatchSize, PollEvery optional
		}

		// start background publisher
		go publisher.Run(ctx)

		// on shutdown, close writer so it flushes
		defer func() {
			if err := writer.Close(); err != nil {
				log.Printf("kafka writer close error: %v", err)
			}
		}()
	}

	// --- Search ---

	// --- OpenSearch (for search endpoint) ---

	var searchSvc *search.Service

	if cfg.EnableSearch {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cfg.OSInsecure, // dev only
			},
		}

		osClient, err := opensearch.NewClient(opensearch.Config{
			Addresses: []string{cfg.OpenSearchURL},
			Username:  cfg.OSUser,
			Password:  cfg.OSPass,
			Transport: tr,
		})
		if err != nil {
			log.Fatal(err)
		}

		searchSvc = search.NewService(osClient, "pairs_v1")
	}

	// --- Provider ---

	httpClient := providers.DefaultHTTPClient()

	// OpenRouter
	provider := providers.OpenRouterProvider(httpClient)

	// Gemini
	// ctx := context.Background()
	// genClient, err := providers.NewGeminiClient(ctx)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// Stub
	// Inject provider function into dispatcher.
	// provider := providers.GeminiProvider(genClient, "gemini-2.5-flash")

	// Provider
	// provider := providers.StubProvider(800 * time.Millisecond)

	// --- Core server (worker pool) ---
	dispatchSvc := dispatcher.New(cfg.QueueSize, cfg.WorkerCount, provider)

	// --- Voting service ---
	voteSvc := voting.NewService(dbpool)

	// --- HTTP API ---

	opts := []api.Option{api.WithVoting(voteSvc)}
	if searchSvc != nil {
		opts = append(opts, api.WithSearch(searchSvc))
	}
	if rdb != nil {
		lim := ratelimit.NewRedisFixedWindowLimiter(
			rdb,
			30,
			time.Minute,
			ratelimit.KeyByIPOrHeader("X-Voter-Id"),
		)
		lim.Prefix = "crowdaudit:rl"

		opts = append(opts, api.WithInferMiddleware(lim.Middleware))
	}

	h := api.New(dispatchSvc, opts...)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           h.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start
	go func() {
		log.Println("listening on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	cancel() // stops outbox publisher goroutine

	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()

	_ = server.Shutdown(shutdownCtx)
	dispatchSvc.Shutdown()

	log.Println("shutdown complete")
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
