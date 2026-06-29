package storage

import (
	"context"
	"errors"
)

// LanceDBVectorAdapter provides a disabled implementation of VectorAdapter for Windows.
// LanceDB Go bindings require CGO and pre-compiled native libraries which are missing
// for Windows in the official releases.
type LanceDBVectorAdapter struct{}

func NewLanceDBVectorAdapter(dbPath string) (*LanceDBVectorAdapter, error) {
	return nil, errors.New("lancedb is natively unsupported on Windows due to missing C-FFI binaries in lancedb-go releases. Please use SQLite for local Windows development or run the application in a Linux environment (Docker/WSL)")
}

func (l *LanceDBVectorAdapter) Insert(ctx context.Context, collection string, vectors [][]float32, metadata []map[string]interface{}) error {
	return errors.New("lancedb vector adapter is disabled on Windows")
}

func (l *LanceDBVectorAdapter) Search(ctx context.Context, collection string, query []float32, limit int) ([]map[string]interface{}, error) {
	return nil, errors.New("lancedb vector adapter is disabled on Windows")
}
