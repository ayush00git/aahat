// Package app wires the data source, store and notifier into the HTTP API.
// Both entry points (local server and Lambda) go through here.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/ayush00git/aahat/api/internal/alert"
	"github.com/ayush00git/aahat/api/internal/data"
	"github.com/ayush00git/aahat/api/internal/httpapi"
	"github.com/ayush00git/aahat/api/internal/store"
)

type Backends struct {
	Data          data.DataStore
	Store         store.Store
	Notifier      alert.Notifier
	WebhookSecret string
	Logger        *slog.Logger
}

// Handler builds the API handler.
func Handler(b Backends) http.Handler {
	catalog := data.NewCatalog(b.Data)
	svc := alert.NewService(catalog, b.Store, b.Store, alert.NewDispatcher(b.Notifier))
	return httpapi.New(httpapi.Config{
		Catalog:       catalog,
		Store:         b.Store,
		Alerts:        svc,
		WebhookSecret: []byte(b.WebhookSecret),
		Logger:        b.Logger,
	})
}

// FromEnv picks backends from environment variables:
//
//	AAHAT_DATA_BUCKET, AAHAT_DATA_PREFIX  S3 data root (else AAHAT_DATA_DIR, a local directory)
//	AAHAT_SUBS_TABLE, AAHAT_EVENTS_TABLE  DynamoDB tables (else an in-memory store)
//	AAHAT_CACHE_TTL                       data cache TTL, e.g. "5m" (default 5m)
//	AAHAT_WEBHOOK_SECRET                  sensor webhook HMAC secret
func FromEnv(ctx context.Context, log *slog.Logger) (Backends, error) {
	b := Backends{
		Notifier:      alert.NewLogNotifier(log),
		WebhookSecret: os.Getenv("AAHAT_WEBHOOK_SECRET"),
		Logger:        log,
	}
	ttl := 5 * time.Minute
	if v := os.Getenv("AAHAT_CACHE_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return b, fmt.Errorf("AAHAT_CACHE_TTL: %w", err)
		}
		ttl = d
	}

	bucket := os.Getenv("AAHAT_DATA_BUCKET")
	subsTable, eventsTable := os.Getenv("AAHAT_SUBS_TABLE"), os.Getenv("AAHAT_EVENTS_TABLE")
	needAWS := bucket != "" || subsTable != ""

	var awsCfg aws.Config
	if needAWS {
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return b, fmt.Errorf("load AWS config: %w", err)
		}
		awsCfg = cfg
	}

	if bucket != "" {
		b.Data = data.NewS3Store(s3.NewFromConfig(awsCfg), bucket, os.Getenv("AAHAT_DATA_PREFIX"))
	} else {
		dir := os.Getenv("AAHAT_DATA_DIR")
		if dir == "" {
			return b, fmt.Errorf("set AAHAT_DATA_BUCKET or AAHAT_DATA_DIR")
		}
		local, err := data.NewLocalStore(dir)
		if err != nil {
			return b, err
		}
		b.Data = local
	}
	b.Data = data.NewCache(b.Data, ttl)

	if subsTable != "" {
		if eventsTable == "" {
			eventsTable = subsTable // single-table setup
		}
		b.Store = store.NewDynamoStore(dynamodb.NewFromConfig(awsCfg), subsTable, eventsTable)
	} else {
		log.Warn("AAHAT_SUBS_TABLE not set: subscriptions and events are kept in memory only")
		b.Store = store.NewMemoryStore()
	}
	return b, nil
}
