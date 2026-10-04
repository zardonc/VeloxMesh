package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const idleBaseline = 30 * time.Second
const idleComponent = 45 * time.Second
const idleQdrant = 120 * time.Second
const memorySnapshotInterval = 5 * time.Second

type idleCondition struct {
	Label     string  `json:"label"`
	Container string  `json:"container,omitempty"`
	Seconds   float64 `json:"seconds"`
}

func idlePlan() []idleCondition {
	return []idleCondition{
		{"baseline", "", idleBaseline.Seconds()},
		{"redis-only", "veloxmesh-test-redis", idleComponent.Seconds()},
		{"after-redis", "", idleBaseline.Seconds()},
		{"postgres-only", "veloxmesh-test-postgres", idleComponent.Seconds()},
		{"after-postgres", "", idleBaseline.Seconds()},
		{"qdrant-only", "veloxmesh-test-qdrant", idleQdrant.Seconds()},
		{"after-qdrant", "", idleBaseline.Seconds()},
	}
}

func (r *runner) recoveryDiagnosis() (result error) {
	if err := r.assertIsolatedStopped(); err != nil {
		return err
	}
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
	plan, err := json.MarshalIndent(idlePlan(), "", "  ")
	if err != nil {
		return err
	}
	if err := r.save("idle-plan.json", plan); err != nil {
		return err
	}
	for _, condition := range idlePlan() {
		if err := r.idleCondition(condition); err != nil {
			return err
		}
	}
	return r.assertIsolatedStopped()
}

func (r *runner) assertIsolatedStopped() error {
	output, err := r.command("docker inspect --format '{{.Name}} {{.State.Running}}' veloxmesh-test-redis veloxmesh-test-qdrant veloxmesh-test-postgres")
	if err != nil {
		return err
	}
	if strings.Contains(string(output), "true") {
		return errors.New("idle isolation requires all three existing test containers stopped")
	}
	return nil
}

func (r *runner) idleCondition(condition idleCondition) error {
	if condition.Container != "" {
		if _, err := r.command("docker start " + condition.Container); err != nil {
			return err
		}
		r.started = append(r.started, condition.Container)
	}
	started := time.Now()
	if err := r.idlePeriod(condition); err != nil {
		return err
	}
	if condition.Container != "" {
		if _, err := r.command("docker stop --time 5 " + condition.Container); err != nil {
			return err
		}
	}
	metadata, err := json.Marshal(map[string]any{"condition": condition, "start": started.UTC(), "end": time.Now().UTC()})
	if err != nil {
		return err
	}
	if err := r.save(condition.Label+"-period.json", metadata); err != nil {
		return err
	}
	fmt.Printf("Idle condition complete: %s\n", condition.Label)
	return nil
}

func (r *runner) idlePeriod(condition idleCondition) error {
	file, err := os.Create(filepath.Join(artifacts, condition.Label+"-memory.jsonl"))
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	deadline := time.Now().Add(time.Duration(condition.Seconds) * time.Second)
	for index := 0; ; index++ {
		if err := r.idleSnapshot(encoder, condition); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(memorySnapshotInterval)
	}
}

func (r *runner) idleSnapshot(encoder *json.Encoder, condition idleCondition) error {
	command := "cat /proc/meminfo /proc/vmstat /proc/pressure/memory"
	if condition.Container != "" {
		command += " && docker exec " + condition.Container + " sh -c 'for f in memory.current memory.stat memory.swap.current memory.events memory.pressure; do echo CGROUP:$f; cat /sys/fs/cgroup/$f || exit; done; for p in /proc/[0-9]*; do name=$(cat $p/comm 2>/dev/null) || continue; case $name in qdrant|postgres|redis-server) echo PROCESS:$name; cat $p/status $p/stat || exit;; esac; done'"
	}
	output, err := r.command(command)
	if saveErr := encoder.Encode(map[string]any{"utc": time.Now().UTC(), "condition": condition.Label,
		"output": string(output), "success": err == nil}); saveErr != nil {
		return saveErr
	}
	return err
}
