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
	handler := app.Handler(app.Backends{
		Data:          data.NewCache(local, cacheTTL),
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
