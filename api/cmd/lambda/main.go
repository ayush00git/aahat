// Command lambda serves the Aahat API from AWS Lambda behind an API Gateway
// HTTP API (payload format 2.0). Backends come from environment variables;
// see app.FromEnv.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/ayush00git/aahat/api/internal/app"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	b, err := app.FromEnv(context.Background(), log)
	if err != nil {
		log.Error("startup failed", "err", err)
		os.Exit(1)
	}
	adapter := httpadapter.NewV2(app.Handler(b))
	lambda.Start(adapter.ProxyWithContext)
}
