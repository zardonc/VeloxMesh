package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
)

const controlledSuite = ".planning/phases/29-semantic-cache-latency-hardening/measurements/memory-upgrade-controlled-20261004/suite.mjs"
const controlledSettlePeriod = 15 * time.Second

func (r *runner) controlledChecks() (result error) {
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
	if err := r.dependencies(); err != nil {
		return err
	}
	fmt.Println("Dependencies ready; 15-second recovery period outside formal test windows")
	time.Sleep(controlledSettlePeriod)
	if err := r.captureTelemetry("controlled", "settled"); err != nil {
		return err
	}
	cmd := exec.Command("node", controlledSuite)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func (r *runner) startControlledMonitor() (func() error, error) {
	if err := os.MkdirAll(artifacts, 0700); err != nil {
		return nil, err
	}
	file, err := os.Create(filepath.Join(artifacts, "vmstat.jsonl"))
	if err != nil {
		return nil, err
	}
	session, err := r.client.NewSession()
	if err != nil {
		file.Close()
		return nil, err
	}
	stream, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		file.Close()
		return nil, err
	}
	session.Stderr = file
	if err := session.Start("exec timeout 1800s vmstat -w -t 1"); err != nil {
		session.Close()
		file.Close()
		return nil, err
	}
	done := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stream)
		encoder := json.NewEncoder(file)
		for scanner.Scan() {
			if err := encoder.Encode(map[string]any{"utc": time.Now().UTC(), "line": scanner.Text()}); err != nil {
				done <- err
				return
			}
		}
		done <- scanner.Err()
	}()
	return func() error {
		_ = session.Signal(ssh.SIGTERM)
		_ = session.Close()
		return errors.Join(<-done, file.Close())
	}, nil
}
