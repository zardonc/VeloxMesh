package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const inventoryRoot = ".planning/phases/29-semantic-cache-latency-hardening/measurements/layout-clean-investigation-20261004"

func main() {
	run := runInventory
	if len(os.Args) > 1 && os.Args[1] == "cleanup" {
		run = cleanupVM
	}
	if len(os.Args) > 1 && os.Args[1] == "recover" {
		run = recoverCleanup
	}
	if len(os.Args) > 1 && os.Args[1] == "experiment" {
		run = runExperiment
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func connectInventory() (*ssh.Client, error) {
	env, err := godotenv.Read(".env.local")
	if err != nil {
		return nil, err
	}
	userDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	callback, err := knownhosts.New(filepath.Join(userDir, ".ssh", "known_hosts"))
	if err != nil {
		return nil, err
	}
	port := env["DEV_SERVER_PORT"]
	if port == "" {
		port = "22"
	}
	return ssh.Dial("tcp", net.JoinHostPort(env["DEV_SERVER_IP"], port), &ssh.ClientConfig{
		User: env["DEV_SERVER_USER"], Auth: []ssh.AuthMethod{ssh.Password(env["DEV_SERVER_PW"])},
		HostKeyCallback: callback, Timeout: 10 * time.Second})
}

func captureInventory(client *ssh.Client, name, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	output, err := session.CombinedOutput(command)
	if saveErr := os.WriteFile(filepath.Join(inventoryRoot, name), output, 0600); saveErr != nil {
		return saveErr
	}
	return err
}

func runInventory() error {
	client, err := connectInventory()
	if err != nil {
		return err
	}
	defer client.Close()
	timer := time.AfterFunc(time.Minute, func() { client.Close() })
	defer timer.Stop()
	commands := map[string]string{
		"vm-preflight.log":           "date -u '+%Y-%m-%dT%H:%M:%SZ' && cat /proc/meminfo /proc/pressure/memory && df -k /var/lib/docker && ss -ltn",
		"containers-preflight.jsonl": "docker ps -a --no-trunc --format '{{json .ID}} {{json .Names}} {{json .Image}} {{json .State}}'",
		"isolated-metadata.jsonl":    "docker inspect --format '{{json .Name}} {{json .Image}} {{json .Config.Image}} {{json .State}} {{json .Mounts}} {{json .HostConfig.Memory}} {{json .HostConfig.NanoCpus}}' veloxmesh-test-redis veloxmesh-test-qdrant veloxmesh-test-postgres",
		"volumes-preflight.jsonl":    "docker volume ls --format '{{json .Name}}'",
		"tmp-preflight.log":          "find /tmp -maxdepth 1 -mindepth 1 -printf '%U\\t%y\\t%s\\t%f\\n'",
	}
	for name, command := range commands {
		if err := captureInventory(client, name, command); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(inventoryRoot, "containers-preflight.jsonl"))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		decoder := json.NewDecoder(strings.NewReader(line))
		var id, name, image, state string
		for _, value := range []*string{&id, &name, &image, &state} {
			if err := decoder.Decode(value); err != nil {
				return err
			}
		}
		fmt.Printf("container=%s state=%s image=%s\n", name, state, image)
	}
	return nil
}
