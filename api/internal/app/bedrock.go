package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/ayush00git/aahat/api/internal/assistant"
	"github.com/ayush00git/aahat/api/internal/data"
	"github.com/ayush00git/aahat/api/internal/research"
)

// Bedrock sets up the Claude client from environment variables; without a model it returns nil
// and the features that need it stay off:
//
//	AAHAT_BEDROCK_MODEL    model or inference profile id, e.g. an "apac." or "global." Claude profile
//	AAHAT_BEDROCK_REGION   region to call Bedrock in (default: the SDK's region, AWS_REGION)
func Bedrock(ctx context.Context) (assistant.Converser, string, error) {
	model := os.Getenv("AAHAT_BEDROCK_MODEL")
	if model == "" {
		return nil, "", nil
	}
	var opts []func(*config.LoadOptions) error
	if region := os.Getenv("AAHAT_BEDROCK_REGION"); region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("load AWS config for Bedrock: %w", err)
	}
	return bedrockruntime.NewFromConfig(cfg), model, nil
}

// JobsDir is where researcher job files live: AAHAT_JOBS_DIR, else <stateDir>/jobs. The worker
// on the server (infra/ec2/research-worker.sh) watches the same directory.
func JobsDir(stateDir string) string {
	if dir := os.Getenv("AAHAT_JOBS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(stateDir, "jobs")
}

// bedrockFeatures builds the /ask handler (nil without a model: the route then answers 503) and
// the researcher API (nil without a jobs directory). api is the finished API handler.
//
//	AAHAT_ASK_PER_MIN   questions allowed per client IP per minute (default 10)
func bedrockFeatures(b Backends, catalog *data.Catalog, api http.Handler) (ask, researcher http.Handler) {
	log := b.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	haveModel := b.Bedrock != nil && b.BedrockModel != ""
	if haveModel {
		svc := &assistant.Service{Client: b.Bedrock, Model: b.BedrockModel, API: api, Log: log}
		if b.Voice != nil { // not a nil *Polly inside a non-nil interface
			svc.Speaker = b.Voice
		}
		perMin, _ := strconv.Atoi(os.Getenv("AAHAT_ASK_PER_MIN"))
		ask = assistant.Handler(svc, perMin)
	} else {
		ask = assistant.NotConfigured()
	}

	if b.JobsDir == "" {
		return ask, nil
	}
	jobs, err := research.NewDirStore(b.JobsDir)
	if err != nil {
		log.Error("researcher API disabled: jobs directory not usable", "dir", b.JobsDir, "err", err)
		return ask, nil
	}
	index := research.Index{Catalog: catalog}
	svc := &research.Service{Store: jobs, Resolver: research.CatalogueResolver{Index: index}, Log: log}
	if haveModel {
		svc.Resolver = research.ClaudeResolver{Client: b.Bedrock, Model: b.BedrockModel, Index: index}
		svc.Summarizer = research.ClaudeSummarizer{Client: b.Bedrock, Model: b.BedrockModel}
	}
	return ask, svc.Handler()
}
