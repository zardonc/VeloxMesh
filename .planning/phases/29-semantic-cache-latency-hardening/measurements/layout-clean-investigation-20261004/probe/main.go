package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type options struct {
	BaseURL        string `json:"base_url,omitempty"`
	StopFile       string `json:"stop_file,omitempty"`
	Action         string `json:"action"`
	APIKey         string `json:"api_key,omitempty"`
	Collections    int    `json:"collections,omitempty"`
	Points         int    `json:"points,omitempty"`
	Dimension      int    `json:"dimension,omitempty"`
	Seed           uint64 `json:"seed,omitempty"`
	PID            int    `json:"pid,omitempty"`
	Seconds        int    `json:"seconds,omitempty"`
	ExpectedDigest string `json:"expected_digest,omitempty"`
	Queries        int    `json:"queries,omitempty"`
}

func main() {
	var config options
	if err := json.NewDecoder(os.Stdin).Decode(&config); err != nil {
		fail(err)
	}
	if err := execute(config); err != nil {
		fail(err)
	}
}

func fail(err error) {
	_ = emit(map[string]any{"type": "error", "utc": time.Now().UTC(), "error": err.Error()})
	os.Exit(1)
}

func emit(value any) error { return json.NewEncoder(os.Stdout).Encode(value) }

func execute(config options) error {
	switch config.Action {
	case "evict":
		return evictVolume()
	case "cleanup-tmp":
		return cleanupTemporary(config)
	case "prepare":
		return prepareFixture(config)
	case "ready":
		return waitReady(config)
	case "observe":
		return observe(config)
	case "gate":
		return quietGate(config)
	case "query":
		return queryFixture(config)
	default:
		return fmt.Errorf("unsupported probe action")
	}
}
