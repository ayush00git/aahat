package data

import (
	"context"
	"errors"
	"log/slog"
)

// Fallback reads from primary and, when primary fails for any reason other than the key not
// existing, from secondary. The server uses it to serve from S3 with the pipeline's own copy
// on disk behind it, so an S3 or network fault does not take the API down.
type Fallback struct {
	primary, secondary DataStore
	log                *slog.Logger
}

func NewFallback(primary, secondary DataStore, log *slog.Logger) *Fallback {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Fallback{primary: primary, secondary: secondary, log: log}
}

func (f *Fallback) Get(ctx context.Context, key string) ([]byte, error) {
	body, err := f.primary.Get(ctx, key)
	if err == nil || errors.Is(err, ErrNotFound) || ctx.Err() != nil {
		return body, err
	}
	f.log.Warn("data: primary store failed, reading the local copy", "key", key, "err", err)
	return f.secondary.Get(ctx, key)
}
