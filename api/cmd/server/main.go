// Command server runs the Aahat API against the pipeline's output directory with a JSON-file
// store (local development, or a single EC2 instance). Delivery channels come from the
// environment, see app.Channels.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/ayush00git/aahat/api/internal/app"
	"github.com/ayush00git/aahat/api/internal/data"
	"github.com/ayush00git/aahat/api/internal/store"
)

func main() {
	addr := flag.String("addr", envOr("AAHAT_ADDR", ":8080"), "listen address")
	dataDir := flag.String("data", envOr("AAHAT_DATA_DIR", "../pipeline/out"), "pipeline output directory (the data root)")
	storeFile := flag.String("store", envOr("AAHAT_STORE_FILE", filepath.Join(os.TempDir(), "aahat-dev", "store.json")),
		"JSON file for subscriptions and events")
	cacheTTL := flag.Duration("cache-ttl", 10*time.Second, "how long to cache data files")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log, *addr, *dataDir, *storeFile, *cacheTTL); err != nil {
		log.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, addr, dataDir, storeFile string, cacheTTL time.Duration) error {
	local, err := data.NewLocalStore(dataDir)
	if err != nil {
		return err
	}
	st, err := store.NewFileStore(storeFile)
	if err != nil {
		return err
	}
	notifier, speaker, pushKey, err := app.Channels(context.Background(), log, filepath.Dir(storeFile))
	if err != nil {
		return err
	}
	bedrock, bedrockModel, err := app.Bedrock(context.Background())
	if err != nil {
		return err
	}
	// With AAHAT_DATA_BUCKET the API serves the copy the pipeline publishes to S3 (prefix
	// AAHAT_DATA_PREFIX), with the directory on disk behind it in case S3 cannot be reached.
	var files data.DataStore = local
	if bucket := os.Getenv("AAHAT_DATA_BUCKET"); bucket != "" {
		awsCfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			return err
		}
		files = data.NewFallback(data.NewS3Store(s3.NewFromConfig(awsCfg), bucket, os.Getenv("AAHAT_DATA_PREFIX")), local, log)
		if cacheTTL < 10*time.Minute {
			cacheTTL = 10 * time.Minute // the bucket changes once per refresh; spare the round trips
		}
		log.Info("serving data from S3", "bucket", bucket, "prefix", os.Getenv("AAHAT_DATA_PREFIX"))
	}
	handler := app.Handler(app.Backends{
		Data:          data.NewCache(files, cacheTTL),
		Store:         st,
		Notifier:      notifier,
		WebhookSecret: os.Getenv("AAHAT_WEBHOOK_SECRET"),
		Logger:        log,
		Voice:         speaker,
		PushPublicKey: pushKey,
		OfficialToken: os.Getenv("AAHAT_OFFICIAL_TOKEN"),
		Bedrock:       bedrock,
		BedrockModel:  bedrockModel,
		JobsDir:       app.JobsDir(filepath.Dir(storeFile)),
	})

	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Info("aahat api listening", "addr", addr, "data", dataDir, "store", storeFile)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
