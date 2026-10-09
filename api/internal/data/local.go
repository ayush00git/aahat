package data

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// LocalStore reads keys from a directory on disk.
type LocalStore struct {
	root *os.Root
}

// NewLocalStore opens dir as the data root. Reads go through os.Root, so a key
// can never escape the directory, even through symlinks.
func NewLocalStore(dir string) (*LocalStore, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open data dir: %w", err)
	}
	return &LocalStore{root: root}, nil
}

func (s *LocalStore) Get(_ context.Context, key string) ([]byte, error) {
	if !fs.ValidPath(key) {
		return nil, ErrNotFound
	}
	b, err := s.root.ReadFile(key)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return b, err
}
