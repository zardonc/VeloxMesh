package main

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// Hold only task-owned dependencies warm across balanced windows. Child runners
// own their own SSH forwarding; this parent restores the original container state.
func (r *runner) batchChecks() error {
	if err := r.dependencies(); err != nil {
		return err
	}
	stages := []string{"safety", "performance", "capacity", "protocol", "regression", "exact-safety"}
	if selected := os.Getenv("SHIP_BATCH_STAGES"); selected != "" {
		stages = strings.Split(selected, ",")
		for _, stage := range stages {
			if !slices.Contains([]string{"safety", "performance", "capacity", "protocol", "regression", "exact-safety", "final-checks"}, stage) {
				return fmt.Errorf("unsupported batch stage")
			}
		}
	}
	var failed bool
	for _, stage := range stages {
		cmd := exec.Command("node", ".planning/phases/29-semantic-cache-latency-hardening/measurements/policy-execution-20261003/suite.mjs", stage)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "batch stage %s: %v\n", stage, err)
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("one or more batch stages failed; raw evidence retained")
	}
	return nil
}
