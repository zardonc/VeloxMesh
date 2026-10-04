package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"
)

const upsertBatch = 128
const fixturePoints = 16542
const fixtureDimensions = 768
const fixtureSeed = 29
const fixtureQueries = 100

type point struct {
	ID      int               `json:"id"`
	Vector  []float32         `json:"vector"`
	Payload map[string]string `json:"payload"`
}

func generatedPoint(config options, index int) point {
	random := rand.New(rand.NewPCG(config.Seed, uint64(index+1)))
	vector := make([]float32, config.Dimension)
	for j := range vector {
		vector[j] = random.Float32()*2 - 1
	}
	return point{index + 1, vector, map[string]string{"scope": scope(index)}}
}

func prepareFixture(config options) error {
	if (config.Collections != 1 && config.Collections != scopeCount) || config.Points != fixturePoints || config.Dimension != fixtureDimensions || config.Seed != fixtureSeed {
		return fmt.Errorf("fixture does not match preregistration")
	}
	a, err := newAPI(config)
	if err != nil {
		return err
	}
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	for index := 0; index < config.Points; index++ {
		if err := encoder.Encode(generatedPoint(config, index)); err != nil {
			return err
		}
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	for index := 0; index < config.Collections; index++ {
		name := collection(config, index)
		if err := createCollection(a, name); err != nil {
			return err
		}
		if err := insertCollection(a, index); err != nil {
			return err
		}
		if err := emit(map[string]any{"type": "prepared-collection", "name": name, "utc": time.Now().UTC()}); err != nil {
			return err
		}
	}
	if err := validateFixture(a); err != nil {
		return err
	}
	return emit(map[string]any{"type": "fixture-complete", "collections": config.Collections, "points": config.Points, "dimension": config.Dimension, "request_digest": digest})
}

func createCollection(a api, name string) error {
	config := map[string]any{"vectors": map[string]any{"size": fixtureDimensions, "distance": "Cosine", "on_disk": false}, "shard_number": 1, "hnsw_config": map[string]any{"m": 0}, "optimizers_config": map[string]any{"indexing_threshold": 0, "default_segment_number": 2}, "wal_config": map[string]any{"wal_capacity_mb": 32, "wal_segments_ahead": 0}, "on_disk_payload": true}
	if _, err := a.request("PUT", "/collections/"+name, config); err != nil {
		return err
	}
	_, err := a.request("PUT", "/collections/"+name+"/index?wait=true", map[string]any{"field_name": "scope", "field_schema": "keyword"})
	return err
}

func insertCollection(a api, layoutIndex int) error {
	batch := make([]point, 0, upsertBatch)
	start, step := layoutIndex, a.Config.Collections
	for index := start; index < a.Config.Points; index += step {
		batch = append(batch, generatedPoint(a.Config, index))
		if len(batch) < upsertBatch && index+step < a.Config.Points {
			continue
		}
		if _, err := a.request("PUT", "/collections/"+collection(a.Config, layoutIndex)+"/points?wait=true", map[string]any{"points": batch}); err != nil {
			return err
		}
		batch = make([]point, 0, upsertBatch)
	}
	return nil
}

func validateFixture(a api) error {
	total := 0
	for index := 0; index < a.Config.Collections; index++ {
		result, err := a.request("GET", "/collections/"+collection(a.Config, index), nil)
		if err != nil {
			return err
		}
		var info struct {
			Points  int `json:"points_count"`
			Indexed int `json:"indexed_vectors_count"`
		}
		if err := json.Unmarshal(result, &info); err != nil {
			return err
		}
		expected := (a.Config.Points + a.Config.Collections - 1 - index) / a.Config.Collections
		if info.Points != expected || info.Indexed != 0 {
			return fmt.Errorf("fixture count/index mismatch: %s", result)
		}
		if err := emit(map[string]any{"type": "collection-validation", "name": collection(a.Config, index), "detail": result}); err != nil {
			return err
		}
		total += info.Points
	}
	if total != a.Config.Points {
		return fmt.Errorf("total point mismatch")
	}
	return nil
}

func queryFixture(config options) error {
	if config.Queries != fixtureQueries {
		return fmt.Errorf("query count mismatch")
	}
	a, err := newAPI(config)
	if err != nil {
		return err
	}
	for index := 0; index < config.Queries; index++ {
		generated := generatedPoint(config, index)
		body := map[string]any{"query": generated.Vector, "limit": 1, "with_payload": true, "filter": map[string]any{"must": []any{map[string]any{"key": "scope", "match": map[string]string{"value": scope(index)}}}}}
		start := time.Now()
		result, err := a.request("POST", "/collections/"+collection(config, index)+"/points/query", body)
		if err != nil {
			return err
		}
		var matches struct {
			Points []struct {
				ID      int               `json:"id"`
				Payload map[string]string `json:"payload"`
				Score   float64           `json:"score"`
			} `json:"points"`
		}
		if err := json.Unmarshal(result, &matches); err != nil {
			return err
		}
		if len(matches.Points) != 1 || matches.Points[0].ID != generated.ID || matches.Points[0].Payload["scope"] != scope(index) {
			return fmt.Errorf("scoped self-query mismatch: %s", result)
		}
		if err := emit(map[string]any{"type": "query", "utc": time.Now().UTC(), "index": index, "elapsed_ms": float64(time.Since(start).Microseconds()) / 1000, "result": result}); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}
