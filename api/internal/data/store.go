// Package data reads the files the pipeline publishes (the lake index and the
// per-lake JSON and GeoJSON) from a local directory or an S3 prefix.
package data

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a key (or a lake) does not exist.
var ErrNotFound = errors.New("not found")

// DataStore fetches one published file by its key, a slash-separated path
// relative to the data root such as "lakes/index.json".
type DataStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
}
