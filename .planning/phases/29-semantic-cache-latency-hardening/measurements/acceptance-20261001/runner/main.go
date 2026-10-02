package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const artifacts = ".planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001"
const remoteBinary = "/tmp/veloxmesh-phase29-acceptance-20261001.test"
const testLimit = 60 * time.Second

type runner struct {
	client    *ssh.Client
	env       map[string]string
	started   []string
	listeners []net.Listener
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	env, err := godotenv.Read(".env")
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if env == nil {
		env = map[string]string{}
	}
	local, err := godotenv.Read(".env.local")
	if err != nil {
		return err
	}
	for key, value := range local {
		env[key] = value
	}
	if len(os.Args) > 1 && os.Args[1] == "audit" {
		return audit(env)
	}
	for _, key := range []string{"PHASE29_MODEL", "PHASE29_EMBEDDING_BASE_URL", "PHASE29_READ_TIMEOUT", "PHASE29_READ_CONCURRENCY", "PHASE29_WRITE_WORKERS", "PHASE29_QUEUE_CAPACITY", "PHASE29_WRITE_TIMEOUT", "PHASE29_SHUTDOWN_GRACE"} {
		if os.Getenv(key) == "" {
			return fmt.Errorf("explicit test input required: %s", key)
		}
	}
	client, err := connect(env)
	if err != nil {
		return err
	}
	r := &runner{client: client, env: env}
	defer r.cleanup()
	if err := r.dependencies(); err != nil {
		return err
	}
	if err := r.forward(); err != nil {
		return err
	}
	if err := r.localChecks(); err != nil {
		return err
	}
	return r.liveChecks()
}

func connect(env map[string]string) (*ssh.Client, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	callback, err := knownhosts.New(filepath.Join(home, ".ssh", "known_hosts"))
	if err != nil {
		return nil, err
	}
	port := env["DEV_SERVER_PORT"]
	if port == "" {
		port = "22"
	}
	cfg := &ssh.ClientConfig{User: env["DEV_SERVER_USER"], Auth: []ssh.AuthMethod{ssh.Password(env["DEV_SERVER_PW"])}, HostKeyCallback: callback, Timeout: 10 * time.Second}
	return ssh.Dial("tcp", net.JoinHostPort(env["DEV_SERVER_IP"], port), cfg)
}

func (r *runner) command(command string) ([]byte, error) {
	session, err := r.client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.CombinedOutput(command)
}

func (r *runner) dependencies() error {
	for _, name := range []string{"veloxmesh-test-redis", "veloxmesh-test-qdrant", "veloxmesh-test-postgres"} {
		output, err := r.command("docker inspect --format '{{.State.Running}}' " + name)
		if err != nil {
			return fmt.Errorf("dependency inspection %s: %w", name, err)
		}
		if strings.TrimSpace(string(output)) == "false" {
			if _, err := r.command("docker start " + name); err != nil {
				return err
			}
			r.started = append(r.started, name)
		}
		fmt.Printf("dependency ready: %s\n", name)
	}
	qdrant, err := r.containerEnv("veloxmesh-test-qdrant")
	if err != nil {
		return err
	}
	r.env["QDRANT_API_KEY"] = qdrant["QDRANT__SERVICE__API_KEY"]
	postgres, err := r.containerEnv("veloxmesh-test-postgres")
	if err != nil {
		return err
	}
	r.env["POSTGRES_USER"] = postgres["POSTGRES_USER"]
	r.env["POSTGRES_PASSWORD"] = postgres["POSTGRES_PASSWORD"]
	r.env["POSTGRES_DB"] = postgres["POSTGRES_DB"]
	_, err = r.command("docker exec veloxmesh-test-redis redis-cli ping && docker exec veloxmesh-test-postgres pg_isready")
	return err
}

func (r *runner) containerEnv(name string) (map[string]string, error) {
	output, err := r.command("docker inspect --format '{{json .Config.Env}}' " + name)
	if err != nil {
		return nil, err
	}
	var values []string
	if err := json.Unmarshal(output, &values); err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result, nil
}

func (r *runner) forward() error {
	for _, port := range []string{"6379", "6334", "5432"} {
		listener, err := net.Listen("tcp", "127.0.0.1:"+port)
		if err != nil {
			return err
		}
		r.listeners = append(r.listeners, listener)
		go serve(listener, func() (net.Conn, error) { return r.client.Dial("tcp", "127.0.0.1:"+port) })
	}
	listener, err := r.client.Listen("tcp", "127.0.0.1:11234")
	if err != nil {
		return err
	}
	r.listeners = append(r.listeners, listener)
	go serve(listener, func() (net.Conn, error) { return net.DialTimeout("tcp", "127.0.0.1:1234", 5*time.Second) })
	return nil
}

func serve(listener net.Listener, dial func() (net.Conn, error)) {
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer connection.Close()
			upstream, err := dial()
			if err != nil {
				fmt.Fprintln(os.Stderr, "forward connection:", err)
				return
			}
			defer upstream.Close()
			var streams sync.WaitGroup
			streams.Add(2)
			go func() { defer streams.Done(); _, _ = io.Copy(upstream, connection); upstream.Close() }()
			go func() { defer streams.Done(); _, _ = io.Copy(connection, upstream); connection.Close() }()
			streams.Wait()
		}()
	}
}

func (r *runner) localChecks() error {
	vars := append(os.Environ(), "REDIS_ADDR=127.0.0.1:6379", "REDIS_PASSWORD=", "QDRANT_ADDR=127.0.0.1:6334", "DEV_SERVER_IP=127.0.0.1", "SANS_BASE_URL=", "SANS_PRIMARY_API_KEY=", "SANS_PRIMARY_MODELS=", "SANS_PRIMARY_DEFAULT_MODEL=")
	for _, key := range []string{"QDRANT_API_KEY", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
		vars = append(vars, key+"="+r.env[key])
	}
	ctx, cancel := context.WithTimeout(context.Background(), testLimit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "-timeout", "60s", "-json", "./...")
	cmd.Env = vars
	output, err := cmd.CombinedOutput()
	if saveErr := r.save("full-backend.jsonl", output); saveErr != nil {
		return saveErr
	}
	fmt.Printf("full backend exit: %v\n", err)
	// Functional acceptance still runs when unrelated integration infrastructure fails.
	return nil
}

func (r *runner) liveChecks() error {
	binary, err := os.Open(filepath.Join(artifacts, "app-linux.test"))
	if err != nil {
		return err
	}
	defer binary.Close()
	session, err := r.client.NewSession()
	if err != nil {
		return err
	}
	session.Stdin = binary
	if err := session.Run("umask 077; cat > " + remoteBinary + "; chmod 700 " + remoteBinary); err != nil {
		session.Close()
		return err
	}
	session.Close()
	var failures []error
	for _, test := range []string{"TestPhase29LocalGatewayAcceptance", "TestPhase29LocalEmbeddingFault", "TestPhase29LocalQueueBurst"} {
		if err := r.liveTest(test); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (r *runner) liveTest(test string) error {
	session, err := r.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	input := map[string]string{}
	for _, key := range []string{"SANS_BASE_URL", "SANS_PRIMARY_API_KEY", "SANS_PRIMARY_DEFAULT_MODEL", "QDRANT_API_KEY"} {
		input[key] = r.env[key]
	}
	for _, key := range []string{"PHASE29_EMBEDDING_API_KEY", "PHASE29_EMBEDDING_BASE_URL", "PHASE29_READ_TIMEOUT", "PHASE29_READ_CONCURRENCY", "PHASE29_WRITE_WORKERS", "PHASE29_QUEUE_CAPACITY", "PHASE29_WRITE_TIMEOUT", "PHASE29_SHUTDOWN_GRACE"} {
		input[key] = os.Getenv(key)
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	session.Stdin = strings.NewReader(string(payload))
	model := strings.ReplaceAll(os.Getenv("PHASE29_MODEL"), "'", "'\"'\"'")
	command := "PHASE29_MODEL='" + model + "' timeout --signal=TERM --kill-after=2s 60s " + remoteBinary + " -test.run '^" + test + "$' -test.timeout 60s -test.v"
	output, err := session.CombinedOutput(command)
	if saveErr := r.save(test+".log", output); saveErr != nil {
		return saveErr
	}
	fmt.Printf("%s exit: %v\n", test, err)
	if err != nil {
		return fmt.Errorf("%s: %w", test, err)
	}
	return nil
}

func (r *runner) save(name string, output []byte) error {
	redacted := string(output)
	for key, value := range r.env {
		upper := strings.ToUpper(key)
		if len(value) >= 4 && (strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "API_KEY") || upper == "DEV_SERVER_PW") {
			redacted = strings.ReplaceAll(redacted, value, "[REDACTED]")
		}
	}
	return os.WriteFile(filepath.Join(artifacts, name), []byte(redacted), 0600)
}

func (r *runner) cleanup() {
	for _, listener := range r.listeners {
		_ = listener.Close()
	}
	for _, name := range r.started {
		if _, err := r.command("docker stop --time 5 " + name); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", name, err)
		}
	}
	output, err := r.command("docker inspect --format '{{.Name}} {{.State.Running}}' veloxmesh-test-redis veloxmesh-test-qdrant veloxmesh-test-postgres")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cleanup verification:", err)
	} else {
		fmt.Print(string(output))
	}
	_ = r.client.Close()
}

func audit(env map[string]string) error {
	files, err := os.ReadDir(artifacts)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.IsDir() || (filepath.Ext(file.Name()) != ".jsonl" && filepath.Ext(file.Name()) != ".log") {
			continue
		}
		output, err := os.ReadFile(filepath.Join(artifacts, file.Name()))
		if err != nil {
			return err
		}
		for key, value := range env {
			secret := strings.Contains(key, "KEY") || strings.Contains(key, "PASSWORD") || strings.Contains(key, "SECRET") || strings.Contains(key, "TOKEN") || key == "DEV_SERVER_PW"
			if secret && len(value) >= 4 && strings.Contains(string(output), value) {
				return fmt.Errorf("credential audit failed: %s", file.Name())
			}
		}
	}
	fmt.Println("credential audit passed")
	return nil
}
