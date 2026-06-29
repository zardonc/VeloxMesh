//go:build !lancedb

package storage

import (
	"context"
	"errors"
)

type LanceDBVectorAdapter struct{}

func NewLanceDBVectorAdapter(dbPath string) (*LanceDBVectorAdapter, error) {
	return nil, errors.New("lancedb is not enabled in this build. Compile with -tags lancedb to enable it")
}

func (l *LanceDBVectorAdapter) Insert(ctx context.Context, collection string, vectors [][]float32, metadata []map[string]interface{}) error {
	return errors.New("lancedb vector adapter is disabled")
}

func (l *LanceDBVectorAdapter) Search(ctx context.Context, collection string, query []float32, limit int) ([]map[string]interface{}, error) {
	return nil, errors.New("lancedb vector adapter is disabled")
}

func (l *LanceDBVectorAdapter) Ping(ctx context.Context) error {
	return errors.New("lancedb vector adapter is disabled")
}

func (l *LanceDBVectorAdapter) Delete(ctx context.Context, collection string, filter map[string]interface{}) error {
	return errors.New("lancedb vector adapter is disabled")
}
