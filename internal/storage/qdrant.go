package storage

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
)

type QdrantVectorAdapter struct {
	client *qdrant.Client
}

func NewQdrantVectorAdapter(addr string, apiKey string) (*QdrantVectorAdapter, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
		portStr = "6334"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port in qdrant addr: %w", err)
	}

	client, err := qdrant.NewClient(&qdrant.Config{
		Host:   host,
		Port:   port,
		APIKey: apiKey,
		UseTLS: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create qdrant client: %w", err)
	}

	// Ping to verify connection
	if _, err := client.HealthCheck(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to qdrant: %w", err)
	}

	return &QdrantVectorAdapter{
		client: client,
	}, nil
}

func (q *QdrantVectorAdapter) Insert(ctx context.Context, collection string, vectors [][]float32, metadata []map[string]interface{}) error {
	if len(vectors) == 0 {
		return nil
	}
	
	// Ensure collection exists (lazy creation)
	exists, err := q.client.CollectionExists(ctx, collection)
	if err != nil {
		return fmt.Errorf("failed to check if collection exists: %w", err)
	}
	if !exists {
		// Use vector dimension from the first vector
		dimension := uint64(len(vectors[0]))
		err = q.client.CreateCollection(ctx, &qdrant.CreateCollection{
			CollectionName: collection,
			VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
				Size:     dimension,
				Distance: qdrant.Distance_Cosine,
			}),
		})
		if err != nil {
			return fmt.Errorf("failed to create collection: %w", err)
		}
	}

	points := make([]*qdrant.PointStruct, len(vectors))
	for i, vec := range vectors {
		id := uuid.New().String()
		
		payload := make(map[string]*qdrant.Value)
		if i < len(metadata) {
			for k, v := range metadata[i] {
				switch val := v.(type) {
				case string:
					payload[k] = qdrant.NewValueString(val)
				case int:
					payload[k] = qdrant.NewValueInt(int64(val))
				case int64:
					payload[k] = qdrant.NewValueInt(val)
				case float64:
					payload[k] = qdrant.NewValueDouble(val)
				case bool:
					payload[k] = qdrant.NewValueBool(val)
				}
			}
		}

		points[i] = &qdrant.PointStruct{
			Id:      qdrant.NewIDUUID(id),
			Vectors: qdrant.NewVectors(vec...),
			Payload: payload,
		}
	}

	operationInfo, err := q.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collection,
		Wait:           qdrant.PtrOf(true),
		Points:         points,
	})
	if err != nil {
		return fmt.Errorf("failed to insert points into qdrant: %w", err)
	}

	if operationInfo.Status != qdrant.UpdateStatus_Completed {
		return fmt.Errorf("qdrant upsert did not complete, status: %v", operationInfo.Status)
	}

	return nil
}

func (q *QdrantVectorAdapter) Search(ctx context.Context, collection string, query []float32, limit int) ([]map[string]interface{}, error) {
	results, err := q.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collection,
		Query:          qdrant.NewQuery(query...),
		Limit:          qdrant.PtrOf(uint64(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search points in qdrant: %w", err)
	}

	var mappedResults []map[string]interface{}
	for _, result := range results {
		meta := make(map[string]interface{})
		for k, v := range result.Payload {
			switch val := v.Kind.(type) {
			case *qdrant.Value_StringValue:
				meta[k] = val.StringValue
			case *qdrant.Value_IntegerValue:
				meta[k] = val.IntegerValue
			case *qdrant.Value_DoubleValue:
				meta[k] = val.DoubleValue
			case *qdrant.Value_BoolValue:
				meta[k] = val.BoolValue
			}
		}
		mappedResults = append(mappedResults, meta)
	}

	return mappedResults, nil
}
