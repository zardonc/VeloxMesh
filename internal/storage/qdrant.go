package storage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
)

type QdrantVectorAdapter struct {
	client *qdrant.Client
}

func NewQdrantVectorAdapter(addr string, apiKey string) (*QdrantVectorAdapter, error) {
	cfg, err := qdrantClientConfig(addr, apiKey)
	if err != nil {
		return nil, err
	}
	client, err := qdrant.NewClient(cfg)
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

func qdrantClientConfig(addr string, apiKey string) (*qdrant.Config, error) {
	host, portStr, useTLS, err := qdrantEndpoint(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port in qdrant addr: %w", err)
	}
	return &qdrant.Config{Host: host, Port: port, APIKey: apiKey, UseTLS: useTLS}, nil
}

func qdrantEndpoint(addr string) (string, string, bool, error) {
	if strings.Contains(addr, "://") {
		parsed, err := url.Parse(addr)
		if err != nil {
			return "", "", false, fmt.Errorf("invalid qdrant addr: %w", err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return "", "", false, fmt.Errorf("invalid qdrant addr scheme: %s", parsed.Scheme)
		}
		host := parsed.Hostname()
		if host == "" {
			return "", "", false, fmt.Errorf("invalid qdrant addr: missing host")
		}
		port := parsed.Port()
		if port == "" {
			port = "6334"
		}
		return host, port, parsed.Scheme == "https", nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, "6334", false, nil
	}
	return host, port, false, nil
}

func (q *QdrantVectorAdapter) Insert(ctx context.Context, collection string, vectors [][]float32, metadata []map[string]interface{}) error {
	if len(vectors) == 0 {
		return nil
	}

	if err := q.EnsureCollection(ctx, collection, len(vectors[0])); err != nil {
		return err
	}

	operationInfo, err := q.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collection,
		Wait:           qdrant.PtrOf(true),
		Points:         qdrantPoints(vectors, metadata),
	})
	if err != nil {
		return fmt.Errorf("failed to insert points into qdrant: %w", err)
	}

	if operationInfo.Status != qdrant.UpdateStatus_Completed {
		return fmt.Errorf("qdrant upsert did not complete, status: %v", operationInfo.Status)
	}

	return nil
}

func qdrantPoints(vectors [][]float32, metadata []map[string]interface{}) []*qdrant.PointStruct {
	points := make([]*qdrant.PointStruct, len(vectors))
	for i, vec := range vectors {
		meta := map[string]interface{}{}
		if i < len(metadata) {
			meta = metadata[i]
		}
		points[i] = &qdrant.PointStruct{
			Id:      qdrant.NewIDUUID(uuid.New().String()),
			Vectors: qdrant.NewVectors(vec...),
			Payload: qdrantPayload(meta),
		}
	}
	return points
}

func qdrantPayload(metadata map[string]interface{}) map[string]*qdrant.Value {
	payload := make(map[string]*qdrant.Value)
	for key, value := range metadata {
		if converted := qdrantValue(value); converted != nil {
			payload[key] = converted
		}
	}
	return payload
}

func qdrantValue(value interface{}) *qdrant.Value {
	switch typed := value.(type) {
	case string:
		return qdrant.NewValueString(typed)
	case int:
		return qdrant.NewValueInt(int64(typed))
	case int64:
		return qdrant.NewValueInt(typed)
	case float64:
		return qdrant.NewValueDouble(typed)
	case bool:
		return qdrant.NewValueBool(typed)
	default:
		return nil
	}
}

func (q *QdrantVectorAdapter) EnsureCollection(ctx context.Context, collection string, dimension int) error {
	if dimension < 1 {
		return fmt.Errorf("qdrant collection dimension must be >= 1")
	}
	exists, err := q.client.CollectionExists(ctx, collection)
	if err != nil {
		return fmt.Errorf("failed to check if collection exists: %w", err)
	}
	if exists {
		return nil
	}
	err = q.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(dimension),
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}
	return nil
}

func (q *QdrantVectorAdapter) Search(ctx context.Context, collection string, query []float32, limit int) ([]map[string]interface{}, error) {
	if limit < 1 {
		limit = 1
	}
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
		meta["score"] = float64(result.Score)
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

func (q *QdrantVectorAdapter) Ping(ctx context.Context) error {
	_, err := q.client.HealthCheck(ctx)
	if err != nil {
		return fmt.Errorf("qdrant ping failed: %w", err)
	}
	return nil
}

func (q *QdrantVectorAdapter) Delete(ctx context.Context, collection string, filter map[string]interface{}) error {
	selector, err := qdrantDeleteSelector(filter)
	if err != nil {
		return err
	}
	result, err := q.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collection,
		Wait:           qdrant.PtrOf(true),
		Points:         selector,
	})
	if err != nil {
		return fmt.Errorf("failed to delete points from qdrant: %w", err)
	}
	if result.Status != qdrant.UpdateStatus_Completed {
		return fmt.Errorf("qdrant delete did not complete, status: %v", result.Status)
	}
	return nil
}

func qdrantDeleteSelector(filter map[string]interface{}) (*qdrant.PointsSelector, error) {
	if len(filter) == 0 {
		return nil, errors.New("qdrant delete requires a filter")
	}
	conditions := make([]*qdrant.Condition, 0, len(filter))
	for field, value := range filter {
		condition, err := qdrantMatchCondition(field, value)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, condition)
	}
	return qdrant.NewPointsSelectorFilter(&qdrant.Filter{Must: conditions}), nil
}

func qdrantMatchCondition(field string, value interface{}) (*qdrant.Condition, error) {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil, fmt.Errorf("qdrant delete filter %q cannot be empty", field)
		}
		return qdrant.NewMatch(field, typed), nil
	case int:
		return qdrant.NewMatchInt(field, int64(typed)), nil
	case int64:
		return qdrant.NewMatchInt(field, typed), nil
	case bool:
		return qdrant.NewMatchBool(field, typed), nil
	case float64:
		return qdrant.NewRange(field, &qdrant.Range{Gte: &typed, Lte: &typed}), nil
	default:
		return nil, fmt.Errorf("qdrant delete unsupported filter %q type %T", field, value)
	}
}
