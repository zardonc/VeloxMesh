//go:build phase29preflight

package app

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/storage"
)

const liveProcessDeadline = 15 * time.Second

type liveQdrantChild struct {
	command *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Scanner
	log     bytes.Buffer
}

func TestLiveQdrantCrossProcess(t *testing.T) {
	liveEnvironment(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveProcessDeadline)
	defer cancel()
	collection := fmt.Sprintf("cross_process_%d", time.Now().UnixNano())
	children := []*liveQdrantChild{}
	for range 2 {
		child := startLiveQdrantChild(t, ctx, executable)
		children = append(children, child)
	}
	for _, child := range children {
		liveAwaitChildReady(t, child)
	}
	for _, child := range children {
		fmt.Fprintln(child.input, collection)
		child.input.Close()
	}
	for _, child := range children {
		for child.output.Scan() {
			fmt.Fprintln(&child.log, child.output.Text())
		}
		err := child.command.Wait()
		t.Log(child.log.String())
		if err != nil || child.output.Err() != nil {
			t.Fatalf("cross-process creation: %v scanner=%v", err, child.output.Err())
		}
	}
	shipLogJSON(t, map[string]any{"type": "cross_process_collection", "processes": 2, "creators_per_process": 16, "cold_collection": true})
}

func liveAwaitChildReady(t *testing.T, child *liveQdrantChild) {
	for child.output.Scan() {
		line := child.output.Text()
		fmt.Fprintln(&child.log, line)
		if line == "QDRANT_READY" {
			return
		}
	}
	t.Fatalf("child did not become ready: %s", child.log.String())
}

func startLiveQdrantChild(t *testing.T, ctx context.Context, executable string) *liveQdrantChild {
	t.Helper()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestLiveQdrantChild$", "-test.v", "-test.timeout=10s")
	command.Env = append(os.Environ(), "PHASE29_QDRANT_CHILD=true")
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = command.Stdout
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	return &liveQdrantChild{command: command, input: input, output: bufio.NewScanner(output)}
}

func TestLiveQdrantChild(t *testing.T) {
	if os.Getenv("PHASE29_QDRANT_CHILD") != "true" {
		t.Skip("parent process only")
	}
	adapter, err := storage.NewQdrantVectorAdapter("127.0.0.1:6334", os.Getenv("QDRANT_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("QDRANT_READY")
	reader := bufio.NewReader(os.Stdin)
	collection, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveProcessDeadline)
	defer cancel()
	liveConcurrentEnsure(t, adapter, liveEnsureOptions{ctx: ctx, collection: strings.TrimSpace(collection)})
	if err := adapter.EnsureCollection(ctx, strings.TrimSpace(collection), 4); err == nil {
		t.Fatal("cross-process collection accepted incorrect dimension")
	}
}
