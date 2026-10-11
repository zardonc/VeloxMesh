package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const operationTimeout = 30 * time.Second
const startupTimeout = 120 * time.Second

type api struct {
	Config options
	Client *http.Client
}

func newAPI(config options) (api, error) {
	if config.BaseURL != "http://127.0.0.1:6333" && config.BaseURL != "http://127.0.0.1:16333" {
		return api{}, fmt.Errorf("unsupported fixture URL")
	}
	return api{config, &http.Client{Timeout: operationTimeout}}, nil
}

func (a api) request(method, path string, value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(method, a.Config.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	request.Header.Set("api-key", a.Config.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := a.Client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s HTTP %d: %s", method, path, response.StatusCode, body)
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if path == "/" {
		return body, nil
	}
	return envelope.Result, nil
}

func waitReady(config options) error {
	a, err := newAPI(config)
	if err != nil {
		return err
	}
	start := time.Now()
	for time.Since(start) < startupTimeout {
		version, err := a.request("GET", "/", nil)
		if err == nil {
			return emit(map[string]any{"type": "ready", "utc": time.Now().UTC(), "wait_ms": float64(time.Since(start).Microseconds()) / 1000, "version": version})
		}
		if err := emit(map[string]any{"type": "readiness-poll", "utc": time.Now().UTC(), "error": err.Error()}); err != nil {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("readiness deadline exceeded")
}
