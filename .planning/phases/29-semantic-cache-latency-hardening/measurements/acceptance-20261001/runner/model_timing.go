package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

type modelEvent struct {
	Timestamp int64 `json:"timestamp"`
	Data      struct {
		Type   string     `json:"type"`
		Model  string     `json:"modelIdentifier"`
		Output string     `json:"output"`
		Stats  modelStats `json:"stats"`
	} `json:"data"`
}

type modelStats struct {
	StopReason        string  `json:"stopReason"`
	TokensPerSecond   float64 `json:"tokensPerSecond"`
	FirstTokenSeconds float64 `json:"timeToFirstTokenSec"`
	TotalSeconds      float64 `json:"totalTimeSec"`
	PromptTokens      int     `json:"promptTokensCount"`
	PredictedTokens   int     `json:"predictedTokensCount"`
	TotalTokens       int     `json:"totalTokensCount"`
}

func startModelTiming() (func() error, error) {
	file, err := os.Create(filepath.Join(artifacts, "model-timing.jsonl"))
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("lms", "log", "stream", "--json", "--stats", "--source", "model", "--filter", "output")
	stream, err := cmd.StdoutPipe()
	if err != nil {
		file.Close()
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		file.Close()
		return nil, err
	}
	done := make(chan error, 1)
	var stopping atomic.Bool
	go func() {
		parseErr := copyModelTiming(bufio.NewScanner(stream), json.NewEncoder(file))
		waitErr := cmd.Wait()
		if stopping.Load() {
			waitErr = nil
		}
		done <- errors.Join(parseErr, waitErr)
	}()
	return func() error {
		stopping.Store(true)
		killErr := cmd.Process.Kill()
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
		select {
		case err := <-done:
			return errors.Join(killErr, err, file.Close())
		case <-time.After(5 * time.Second):
			return errors.Join(killErr, errors.New("model timing subscriber cleanup timeout"))
		}
	}, nil
}

func copyModelTiming(scanner *bufio.Scanner, encoder *json.Encoder) error {
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		} // CLI startup banner; never retain text.
		var event modelEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("model timing JSON parse: %w", err)
		}
		if event.Data.Type != "llm.prediction.output" || event.Data.Model != os.Getenv("SHIP_LOCAL_MODEL") {
			continue
		}
		if event.Data.Stats.TotalTokens <= 0 || event.Data.Stats.TotalSeconds <= 0 {
			return errors.New("model timing output lacks usable token/duration statistics")
		}
		hash := sha256.Sum256([]byte(event.Data.Output))
		if err := encoder.Encode(map[string]any{"timestamp_ms": event.Timestamp, "observed_utc": time.Now().UTC(),
			"model": event.Data.Model, "answer_hash": hex.EncodeToString(hash[:]), "stats": event.Data.Stats}); err != nil {
			return err
		}
	}
	return scanner.Err()
}
