package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const primaryBlocks = 6

type primaryWindow struct {
	Block    int    `json:"block"`
	Position int    `json:"position"`
	Warmups  int    `json:"warmups"`
	Test     string `json:"test"`
}

func primaryPlan() []primaryWindow {
	var windows []primaryWindow
	oneCount, eightCount := 0, 0
	for block := 1; block <= primaryBlocks; block++ {
		order := []int{1, 8, 8, 1}
		if block%2 == 0 {
			order = []int{8, 1, 1, 8}
		}
		for position, count := range order {
			arm, ordinal := "One", oneCount+1
			if count == 8 {
				arm = "Eight"
				eightCount++
				ordinal = eightCount
			} else {
				oneCount++
			}
			windows = append(windows, primaryWindow{block, position + 1, count,
				fmt.Sprintf("TestLivePrimary%s%02d", arm, ordinal)})
		}
	}
	return windows
}

func (r *runner) warmupChecks() (result error) {
	stopVM, err := r.startControlledMonitor()
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, stopVM()) }()
	stopHost, err := startHostMonitor()
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, stopHost()) }()
	stopLM, err := startModelTiming()
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, stopLM()) }()
	if err := r.dependencies(); err != nil {
		return err
	}
	if err := r.forward(); err != nil {
		return err
	}
	if err := r.uploadBinary(); err != nil {
		return err
	}
	if err := r.collectionNames("collections-before.json"); err != nil {
		return err
	}
	if err := r.recoveryGate(); err != nil {
		return err
	}
	if err := r.executePrimaryPlan(); err != nil {
		return err
	}
	return r.collectionNames("collections-after.json")
}

func (r *runner) executePrimaryPlan() error {
	windows := primaryPlan()
	plan := map[string]any{"created": time.Now().UTC(), "windows": windows, "cache": "off",
		"count": 100, "interval_ms": 125, "ceiling": 4, "quiet_ms": 2000,
		"upload": "once before recovery", "retry": false, "protocol": "balanced ABBA/BAAB; retain all early requests"}
	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	if err := r.save("test-plan.json", payload); err != nil {
		return err
	}
	for _, window := range windows {
		if _, err := os.Stat(artifacts + "/PAUSE"); err == nil {
			return errors.New("pause requested at window boundary")
		}
		fmt.Printf("block=%d position=%d warmups=%d\n", window.Block, window.Position, window.Warmups)
		if err := r.liveTest(window.Test); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) collectionNames(name string) error {
	session, err := r.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = strings.NewReader("api-key: " + r.env["QDRANT_API_KEY"] + "\n")
	output, err := session.CombinedOutput("timeout 5s curl --fail --silent --show-error -H @- http://127.0.0.1:6333/collections")
	if saveErr := r.save(name, output); saveErr != nil {
		return saveErr
	}
	return err
}
