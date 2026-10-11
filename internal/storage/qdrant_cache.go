package storage

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
)

type CacheVectorWrite struct {
	Collection, ID string
	Vector         []float32
	Metadata       map[string]interface{}
}

type CacheVectorWriter interface {
	InsertCacheEntry(context.Context, CacheVectorWrite) error
}

// Only semantic cache replays use stable IDs; other vector consumers keep theirs.
func (q *QdrantVectorAdapter) InsertCacheEntry(ctx context.Context, write CacheVectorWrite) error {
	if write.ID == "" {
		return fmt.Errorf("semantic cache point requires an entry ID")
	}
	if err := q.EnsureCollection(ctx, write.Collection, len(write.Vector)); err != nil {
		return err
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("veloxmesh-semantic-cache:"+write.ID))
	result, err := q.client.Upsert(ctx, &qdrant.UpsertPoints{CollectionName: write.Collection, Wait: qdrant.PtrOf(true), Points: []*qdrant.PointStruct{{Id: qdrant.NewIDUUID(id.String()), Vectors: qdrant.NewVectors(write.Vector...), Payload: qdrantPayload(write.Metadata)}}})
	if err != nil {
		return fmt.Errorf("semantic cache vector upsert: %w", err)
	}
	if result.Status != qdrant.UpdateStatus_Completed {
		return fmt.Errorf("semantic cache upsert incomplete: %v", result.Status)
	}
	return nil
}
