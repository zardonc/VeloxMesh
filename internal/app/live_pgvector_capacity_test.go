//go:build phase29preflight

package app

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/controlstate/postgres"
	"veloxmesh/internal/storage"
)

func TestLivePGVectorCapacity(t *testing.T) {
	env := liveEnvironment(t)
	vector := liveComponentVector(t)
	dsn := liveVectorSchema(t, env["POSTGRES_TEST_DSN"])
	ctx, cancel := context.WithTimeout(context.Background(), liveHTTPTimeout)
	defer cancel()
	const hnswM, hnswConstruction, searchEF = 16, 64, 40
	adapter, err := storage.NewPGVectorAdapter(ctx, dsn, storage.PGVectorOptions{Dimension: len(vector), HNSWM: hnswM, HNSWEFConstruction: hnswConstruction, SearchEF: searchEF})
	if err != nil {
		t.Fatal(err)
	}
	// The isolated backend process owns and closes the adapter's sockets on exit.
	repo, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	const collection = "component_live_pgvector"
	liveComponentWindows(t, liveComponentWork{name: "pgvector_insert_confirmed", run: func(ctx context.Context, index int) error {
		return adapter.Insert(ctx, collection, [][]float32{vector}, []map[string]interface{}{{"id": fmt.Sprint("real-vector-", index)}})
	}})
	liveComponentWindows(t, liveComponentWork{name: "pgvector_query_populated", run: func(ctx context.Context, _ int) error {
		results, err := adapter.Search(ctx, collection, vector, 10)
		if err == nil && (len(results) == 0 || results[0]["score"].(float64) < .99) {
			return fmt.Errorf("real pgvector round-trip mismatch")
		}
		return err
	}})
	shipLogJSON(t, map[string]any{"type": "pgvector_schema", "dimension": len(vector), "control_state_migrated": true, "isolated": true})
}

func liveVectorSchema(t *testing.T, dsn string) string {
	if dsn == "" {
		t.Fatal("isolated real PostgreSQL DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveComponentDeadline)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema := fmt.Sprint("phase29_vector_", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quotePostgresIdent(schema)); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema+",public")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
