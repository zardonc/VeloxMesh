package predictor

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestWorkerProcessDoesNotStartPerEnsure(t *testing.T) {
	process := helperProcess(t, "sleep", 50*time.Millisecond)
	if err := process.Ensure(context.Background()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if err := process.Ensure(context.Background()); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if process.StartCount() != 1 {
		t.Fatalf("expected one long-lived process, got %d starts", process.StartCount())
	}
	_ = process.Stop()
}

func TestWorkerProcessRestartsAfterBackoff(t *testing.T) {
	process := helperProcess(t, "exit", 50*time.Millisecond)
	if err := process.Ensure(context.Background()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !waitForBackoff(process) {
		t.Fatalf("expected backoff state")
	}
	time.Sleep(60 * time.Millisecond)
	if err := process.Ensure(context.Background()); err != nil {
		t.Fatalf("Ensure after backoff: %v", err)
	}
	if process.StartCount() != 2 {
		t.Fatalf("expected restart after backoff, got %d starts", process.StartCount())
	}
}

func waitForBackoff(process *WorkerProcess) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := process.Ensure(context.Background())
		if errors.Is(err, ErrRestartBackoff) {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

func TestWorkerProcessHelper(t *testing.T) {
	if os.Getenv("PREDICTOR_PROCESS_HELPER") != "1" {
		return
	}
	mode := os.Getenv("PREDICTOR_PROCESS_MODE")
	if mode == "sleep" {
		time.Sleep(5 * time.Second)
	}
	os.Exit(0)
}

func helperProcess(t *testing.T, mode string, backoff time.Duration) *WorkerProcess {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	return &WorkerProcess{
		Command: exe,
		Args:    []string{"-test.run=TestWorkerProcessHelper"},
		Env:     []string{"PREDICTOR_PROCESS_HELPER=1", "PREDICTOR_PROCESS_MODE=" + mode},
		Backoff: backoff,
	}
}
