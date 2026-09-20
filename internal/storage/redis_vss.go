package storage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"veloxmesh/internal/redisconn"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type RedisVSSVectorAdapter struct {
	client    *redis.Client
	namespace string
}

func NewRedisVSSVectorAdapter(ctx context.Context, addr, password string, db int, namespace string) (*RedisVSSVectorAdapter, error) {
	if namespace == "" {
		return nil, errors.New("redis namespace must be configured")
	}

	opts, err := redisconn.Options(addr, password, db)
	if err != nil {
		return nil, err
	}
	redisconn.WarnPlaintextCredentials(nil, "redis_vss", opts)
	client := redis.NewClient(opts)

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	// Verify RediSearch module is loaded
	modules, err := client.Do(ctx, "MODULE", "LIST").Result()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to list redis modules: %w", err)
	}

	searchFound := false
	if mods, ok := modules.([]interface{}); ok {
		for _, m := range mods {
			if modFields, ok := m.([]interface{}); ok && len(modFields) >= 2 {
				name, okName := modFields[0].(string)
				val, okVal := modFields[1].(string)
				if !okName && !okVal { // could be []byte
					if nameBytes, ok := modFields[0].([]byte); ok {
						name = string(nameBytes)
					}
					if valBytes, ok := modFields[1].([]byte); ok {
						val = string(valBytes)
					}
				}
				if name == "name" && val == "search" {
					searchFound = true
					break
				}
			}
		}
	}
	// Fallback to checking FT.INFO or just simple capability check if MODULE LIST parsing is flaky
	if !searchFound {
		// Just try a dummy FT.INFO command to check if it's supported
		err := client.Do(ctx, "FT.INFO", "dummy_index").Err()
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "unknown command") {
			client.Close()
			return nil, errors.New("redis stack VSS capability (RediSearch) is unavailable")
		}
	}

	return &RedisVSSVectorAdapter{
		client:    client,
		namespace: namespace,
	}, nil
}

func (r *RedisVSSVectorAdapter) key(collection string, id string) string {
	return fmt.Sprintf("%s:vss:%s:%s", r.namespace, collection, id)
}

func (r *RedisVSSVectorAdapter) indexName(collection string) string {
	return fmt.Sprintf("%s:idx:%s", r.namespace, collection)
}

func (r *RedisVSSVectorAdapter) ensureIndex(ctx context.Context, collection string, dim int) error {
	idx := r.indexName(collection)
	err := r.client.Do(ctx, "FT.INFO", idx).Err()
	if err == nil {
		return nil // exists
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown index name") {
		return fmt.Errorf("failed to check index info: %w", err)
	}

	// Create index
	prefix := fmt.Sprintf("%s:vss:%s:", r.namespace, collection)

	args := []interface{}{
		"FT.CREATE", idx,
		"ON", "HASH",
		"PREFIX", "1", prefix,
		"SCHEMA",
		"vec", "VECTOR", "HNSW", "6", "TYPE", "FLOAT32", "DIM", fmt.Sprintf("%d", dim), "DISTANCE_METRIC", "COSINE",
		"scope", "TAG",
		"model", "TAG",
		"id", "TAG",
	}

	if err := r.client.Do(ctx, args...).Err(); err != nil {
		return fmt.Errorf("failed to create redis vector index: %w", err)
	}
	return nil
}

func (r *RedisVSSVectorAdapter) Insert(ctx context.Context, collection string, vectors [][]float32, metadata []map[string]interface{}) error {
	if len(vectors) == 0 {
		return nil
	}
	dim := len(vectors[0])
	if err := r.ensureIndex(ctx, collection, dim); err != nil {
		return err
	}

	pipe := r.client.Pipeline()
	for i, vec := range vectors {
		id := uuid.New().String()
		if i < len(metadata) {
			if idVal, ok := metadata[i]["id"].(string); ok && idVal != "" {
				id = idVal
			}
		}

		key := r.key(collection, id)

		vecBytes := make([]byte, len(vec)*4)
		for j, v := range vec {
			bits := math.Float32bits(v)
			vecBytes[j*4] = byte(bits)
			vecBytes[j*4+1] = byte(bits >> 8)
			vecBytes[j*4+2] = byte(bits >> 16)
			vecBytes[j*4+3] = byte(bits >> 24)
		}

		fields := map[string]interface{}{
			"vec": vecBytes,
			"id":  id,
		}

		if i < len(metadata) {
			for k, v := range metadata[i] {
				switch val := v.(type) {
				case string:
					fields[k] = val
				case int, int64, float64, bool:
					fields[k] = fmt.Sprintf("%v", val)
				}
			}
		}

		pipe.HSet(ctx, key, fields)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to insert vectors into redis vss: %w", err)
	}

	return nil
}

func (r *RedisVSSVectorAdapter) Search(ctx context.Context, collection string, query []float32, limit int) ([]map[string]interface{}, error) {
	if limit < 1 {
		limit = 1
	}
	idx := r.indexName(collection)

	vecBytes := make([]byte, len(query)*4)
	for j, v := range query {
		bits := math.Float32bits(v)
		vecBytes[j*4] = byte(bits)
		vecBytes[j*4+1] = byte(bits >> 8)
		vecBytes[j*4+2] = byte(bits >> 16)
		vecBytes[j*4+3] = byte(bits >> 24)
	}

	// FT.SEARCH idx "*=>[KNN limit @vec $query_vec AS dist]" PARAMS 2 query_vec <bytes> DIALECT 2
	knnQuery := fmt.Sprintf("*=>[KNN %d @vec $query_vec AS dist]", limit)

	res, err := r.client.Do(ctx, "FT.SEARCH", idx, knnQuery, "PARAMS", "2", "query_vec", vecBytes, "DIALECT", "2").Result()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unknown index name") {
			return nil, nil // Return empty if index doesn't exist
		}
		return nil, fmt.Errorf("failed to search vectors in redis vss: %w", err)
	}

	var results []map[string]interface{}

	if resMap, ok := res.(map[interface{}]interface{}); ok {
		// New map-based format
		total, _ := resMap["total_results"].(int64)
		if total == 0 {
			return nil, nil
		}
		docs, ok := resMap["results"].([]interface{})
		if !ok {
			return nil, nil
		}
		for _, docInf := range docs {
			doc, ok := docInf.(map[interface{}]interface{})
			if !ok {
				continue
			}
			meta := make(map[string]interface{})
			if extra, ok := doc["extra_attributes"].(map[interface{}]interface{}); ok {
				for k, v := range extra {
					kStr := fmt.Sprintf("%v", k)
					if kStr == "dist" {
						setRedisVSSScore(meta, v)
					} else if kStr != "vec" {
						if vBytes, isBytes := v.([]byte); isBytes {
							meta[kStr] = string(vBytes)
						} else {
							meta[kStr] = v
						}
					}
				}
			}
			results = append(results, meta)
		}
	} else if resSlice, ok := res.([]interface{}); ok {
		// Legacy array-based format
		if len(resSlice) == 0 {
			return nil, nil
		}
		count, ok := resSlice[0].(int64)
		if !ok || count == 0 {
			return nil, nil
		}
		for i := 1; i < len(resSlice); i += 2 {
			if i+1 >= len(resSlice) {
				break
			}
			props, ok := resSlice[i+1].([]interface{})
			if !ok {
				continue
			}

			meta := make(map[string]interface{})
			for j := 0; j < len(props); j += 2 {
				if j+1 >= len(props) {
					break
				}
				kBytes, ok1 := props[j].([]byte)
				vBytes, ok2 := props[j+1].([]byte)
				if ok1 && ok2 {
					k := string(kBytes)
					v := string(vBytes)
					if k == "dist" {
						setRedisVSSScore(meta, vBytes)
					} else if k != "vec" {
						meta[k] = v
					}
				} else if kStr, ok1Str := props[j].(string); ok1Str {
					if vStr, ok2Str := props[j+1].(string); ok2Str {
						if kStr == "dist" {
							setRedisVSSScore(meta, vStr)
						} else if kStr != "vec" {
							meta[kStr] = vStr
						}
					}
				}
			}
			results = append(results, meta)
		}
	}

	return results, nil
}

func setRedisVSSScore(meta map[string]interface{}, dist interface{}) {
	value, ok := redisVSSFloat(dist)
	if !ok {
		return
	}
	meta["score"] = 1 - value
}

func redisVSSFloat(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	case []byte:
		parsed, err := strconv.ParseFloat(string(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func (r *RedisVSSVectorAdapter) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *RedisVSSVectorAdapter) Delete(ctx context.Context, collection string, filter map[string]interface{}) error {
	// A simple delete by ID or scope if provided
	if id, ok := filter["id"].(string); ok && id != "" {
		key := r.key(collection, id)
		return r.client.Del(ctx, key).Err()
	}

	// If a scope is provided, we might be able to search and delete
	if scope, ok := filter["scope"].(string); ok && scope != "" {
		idx := r.indexName(collection)
		res, err := r.client.Do(ctx, "FT.SEARCH", idx, fmt.Sprintf("@scope:{%s}", scope), "NOCONTENT").Result()
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unknown index name") {
				return nil
			}
			return fmt.Errorf("failed to search for deletion: %w", err)
		}

		resSlice, ok := res.([]interface{})
		if !ok || len(resSlice) <= 1 {
			return nil
		}

		pipe := r.client.Pipeline()
		for i := 1; i < len(resSlice); i++ {
			if keyBytes, ok := resSlice[i].([]byte); ok {
				pipe.Del(ctx, string(keyBytes))
			} else if keyStr, ok := resSlice[i].(string); ok {
				pipe.Del(ctx, keyStr)
			}
		}
		_, err = pipe.Exec(ctx)
		return err
	}

	return errors.New("delete unsupported filter for redis vss")
}
