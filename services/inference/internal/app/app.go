package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/api"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/dispatcher"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/infra/dburl"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/infra/redisx"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/obs"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/outbox"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/providers"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/ratelimit"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/search"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/search_conversations"
	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/voting"
)

// In Lambda, you should set ENABLE_OUTBOX_PUBLISHER=false so you don’t start the background goroutine.

// obs.MustRegister(nil) — I used nil to indicate “don’t force prometheus registry here”
// adjust to your actual obs signature.
// In your current code you do obs.MustRegister(prometheus.DefaultRegisterer).
// For Lambda you might still keep it, but it won’t be scraped. Either way, register-once is fine.

type Config struct {
	EnableInfer   bool
	EnableDB      bool
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
	QueueSize     int
	WorkerCount   int
}

func LoadConfigFromEnv() (Config, error) {
	dbURL, err := dburl.Load(context.Background())
	if err != nil {
		// Only error if DB is enabled; for infer-public we allow missing DB
		if os.Getenv("ENABLE_DB") != "false" {
			return Config{}, err
		}
		dbURL = ""
	}

	cfg := Config{
		EnableInfer:   os.Getenv("ENABLE_INFER") != "false",
		EnableDB:      os.Getenv("ENABLE_DB") != "false",
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
		QueueSize:     200,
		WorkerCount:   32,
	}

	if cfg.EnableDB && cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required when ENABLE_DB is true")
	}
	if cfg.EnableRedis && cfg.RedisURL == "" {
		return cfg, fmt.Errorf("REDIS_URL is required when ENABLE_REDIS is true")
	}
	// IMPORTANT: For Lambda, you almost certainly want this OFF.
	if cfg.EnableOutbox && (len(cfg.KafkaBrokers) == 0 || cfg.KafkaBrokers[0] == "") {
		return cfg, fmt.Errorf("KAFKA_BROKERS is required when ENABLE_OUTBOX_PUBLISHER is true")
	}
	if cfg.EnableSearch && cfg.OSPass == "" {
		return cfg, fmt.Errorf("OS_PASSWORD is required when ENABLE_SEARCH is true")
	}

	return cfg, nil
}

type Built struct {
	Handler  http.Handler
	Shutdown func(context.Context)
}

func Build(ctx context.Context, cfg Config) (*Built, error) {
	// Metrics registration: safe to call once per process.
	// Safe in both modes if you wrap in sync.Once at callsite (Lambda init).
	// adjust if your MustRegister requires a registerer
	// NOTE: In Lambda, ensure this is called only once per process (sync.Once in lambda main).
	// adjust if your signature requires a registerer
	// obs.MustRegister(nil)
	obs.MustRegister(prometheus.DefaultRegisterer)

	// --- DB ---
	var dbpool *pgxpool.Pool
	var err error
	if cfg.EnableDB {
		dbpool, err = pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
	}

	// --- Redis ---
	var rdb *redis.Client
	if cfg.EnableRedis {
		rdb, err = redisx.NewClientFromURL(cfg.RedisURL)
		if err != nil {
			if dbpool != nil {
				dbpool.Close()
			}
			return nil, err
		}
	}

	// --- Outbox publisher (Kafka) ---
	// In Lambda: do NOT run background loops. Disable via env for Lambda.
	// Keeping support here for non-Lambda modes.
	// OK for server mode, but disable for Lambda via env
	// --- Kafka outbox publisher (server mode) ---
	var (
		publisherCancel context.CancelFunc
		writer          *kafka.Writer
	)
	if cfg.EnableOutbox {
		if dbpool == nil {
			return nil, fmt.Errorf("EnableOutbox requires EnableDB")
		}

		pubCtx, cancel := context.WithCancel(ctx)
		publisherCancel = cancel

		writer = outbox.NewWriter(cfg.KafkaBrokers)
		publisher := &outbox.Publisher{
			DB:     dbpool,
			Writer: writer,
		}
		go func() {
			_ = publisher.Run(pubCtx) // Run already logs errors per tick
		}()
	}

	// --- Search (OpenSearch) ---
	var searchSvc *search.Service
	if cfg.EnableSearch {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.OSInsecure},
		}
		osClient, err := opensearch.NewClient(opensearch.Config{
			Addresses: []string{cfg.OpenSearchURL},
			Username:  cfg.OSUser,
			Password:  cfg.OSPass,
			Transport: tr,
		})
		if err != nil {
			if publisherCancel != nil {
				publisherCancel()
			}
			if writer != nil {
				_ = writer.Close()
			}
			if rdb != nil {
				_ = rdb.Close()
			}
			if dbpool != nil {
				dbpool.Close()
			}
			return nil, err
		}
		searchSvc = search.NewService(osClient, "pairs_v1")
	}

	var community *search_conversations.CommunityService
	if dbpool != nil {
		community = &search_conversations.CommunityService{DB: dbpool}
	}

	// --- Provider + dispatcher ---
	var dispatchSvc *dispatcher.Server
	if cfg.EnableInfer {
		httpClient := providers.DefaultHTTPClient()
		provider := providers.OpenRouterProvider(httpClient) // needs key
		dispatchSvc = dispatcher.New(cfg.QueueSize, cfg.WorkerCount, provider)
	}

	// --- Voting ---
	var voteSvc *voting.Service
	if dbpool != nil {
		voteSvc = voting.NewService(dbpool)
	}

	// --- HTTP API ---
	opts := []api.Option{}
	if searchSvc != nil {
		opts = append(opts, api.WithSearch(searchSvc))
	}
	if voteSvc != nil {
		opts = append(opts, api.WithVoting(voteSvc))
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

	httpAPI := api.New(dispatchSvc, opts...)

	httpAPI.Community = community

	handler := httpAPI.Routes()

	shutdown := func(shutdownCtx context.Context) {
		// stop publisher first
		if publisherCancel != nil {
			publisherCancel()
		}
		if writer != nil {
			if err := writer.Close(); err != nil {
				log.Printf("kafka writer close error: %v", err)
			}
		}
		dispatchSvc.Shutdown()
		if rdb != nil {
			_ = rdb.Close()
		}
		if dbpool != nil {
			dbpool.Close()
		}
	}

	return &Built{Handler: handler, Shutdown: shutdown}, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
