//go:build phase29preflight

package app

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"veloxmesh/internal/storage"
)

func TestLiveQdrantErrorClassification(t *testing.T) {
	liveEnvironment(t)
	client, err := qdrant.NewClient(&qdrant.Config{Host: "127.0.0.1", Port: 6334, APIKey: os.Getenv("QDRANT_API_KEY")})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), liveRecoveryWait)
	defer cancel()
	collection := fmt.Sprintf("wrong_distance_%d", time.Now().UnixNano())
	err = client.CreateCollection(ctx, &qdrant.CreateCollection{CollectionName: collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{Size: 3, Distance: qdrant.Distance_Dot})})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := storage.NewQdrantVectorAdapter("127.0.0.1:6334", os.Getenv("QDRANT_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.EnsureCollection(ctx, collection, 3); err == nil {
		t.Fatal("wrong distance accepted")
	} else {
		t.Logf("wrong_distance error=%v", err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if err := adapter.EnsureCollection(canceled, collection, 3); err == nil {
		t.Fatal("cancellation swallowed")
	} else {
		t.Logf("canceled error=%v", err)
	}
	badKeyClient, err := qdrant.NewClient(&qdrant.Config{Host: "127.0.0.1", Port: 6334, APIKey: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	defer badKeyClient.Close()
	if _, err := badKeyClient.CollectionExists(ctx, collection); err == nil {
		t.Fatal("incorrect real Qdrant credential accepted")
	} else {
		t.Logf("unauthorized error=%v", err)
	}
}
