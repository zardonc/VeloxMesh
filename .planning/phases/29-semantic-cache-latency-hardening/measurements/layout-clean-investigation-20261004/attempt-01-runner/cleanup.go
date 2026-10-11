package main

import (
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/ssh"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const remoteLayoutDirectory = "/tmp/veloxmesh-phase29-layout-20261004-01a103bf"

type invocation struct {
	Name, Command string
	Input         any
}

func recoverCleanup() error {
	client, err := connectInventory()
	if err != nil {
		return err
	}
	defer client.Close()
	timer := time.AfterFunc(3*time.Minute, func() { client.Close() })
	defer timer.Stop()
	image, err := smallCommand(client, "docker inspect --format '{{.Image}}' veloxmesh-test-postgres")
	if err != nil {
		return err
	}
	if err := invokeProbe(client, invocation{"cleanup-qdrant-second-pass.jsonl", evictionCommand(strings.TrimSpace(image), "veloxmesh-test-qdrant"), map[string]any{"action": "evict"}}); err != nil {
		return err
	}
	return captureInventory(client, "vm-after-cleanup.log", "date -u '+%Y-%m-%dT%H:%M:%SZ' && cat /proc/meminfo /proc/pressure/memory && test ! -e /tmp/veloxmesh-phase29-acceptance-20261001.test && docker ps --format '{{.Names}}'")
}

func cleanupVM() error {
	client, err := connectInventory()
	if err != nil {
		return err
	}
	defer client.Close()
	timer := time.AfterFunc(5*time.Minute, func() { client.Close() })
	defer timer.Stop()
	if err := uploadProbe(client); err != nil {
		return err
	}
	data, err := os.ReadFile(".planning/phases/29-semantic-cache-latency-hardening/measurements/primary-warmup-investigation-20261004/manifest.json")
	if err != nil {
		return err
	}
	var manifest struct {
		Binary map[string]string `json:"binary_sha256"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	digest := manifest.Binary[".tmp/primary-warmup-investigation-20261004/app-linux.test"]
	if len(digest) != 64 {
		return fmt.Errorf("missing retained old binary digest")
	}
	if err := invokeProbe(client, invocation{"cleanup-tmp.jsonl", remoteLayoutDirectory + "/probe", map[string]any{"action": "cleanup-tmp", "expected_digest": digest}}); err != nil {
		return err
	}
	image, err := smallCommand(client, "docker inspect --format '{{.Image}}' veloxmesh-test-postgres")
	if err != nil {
		return err
	}
	image = strings.TrimSpace(image)
	if !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(image) {
		return fmt.Errorf("invalid helper image")
	}
	for _, volume := range []string{"veloxmesh-test-redis", "veloxmesh-test-postgres", "veloxmesh-test-qdrant"} {
		if err := invokeProbe(client, invocation{"cleanup-" + volume + ".jsonl", evictionCommand(image, volume), map[string]any{"action": "evict"}}); err != nil {
			return err
		}
		fmt.Println("verified file-page eviction:", volume)
	}
	return captureInventory(client, "vm-after-cleanup.log", "date -u '+%Y-%m-%dT%H:%M:%SZ' && cat /proc/meminfo /proc/pressure/memory && test ! -e /tmp/veloxmesh-phase29-acceptance-20261001.test && docker ps --format '{{.Names}}'")
}

func evictionCommand(image, volume string) string {
	return "timeout 120s docker run --rm -i --pull never --read-only --network none --cap-drop ALL --cap-add DAC_OVERRIDE --security-opt no-new-privileges --memory 256m --cpus 0.5 --mount type=volume,source=" + volume + ",target=/fixture,readonly --mount type=bind,source=" + remoteLayoutDirectory + "/probe,target=/probe,readonly --entrypoint /probe " + image
}

func uploadProbe(client *ssh.Client) error {
	active, err := smallCommand(client, "docker ps -q")
	if err != nil {
		return err
	}
	if strings.TrimSpace(active) != "" {
		return fmt.Errorf("active container collision; cleanup refused")
	}
	if _, err := smallCommand(client, "test ! -e "+remoteLayoutDirectory+" && mkdir -m 700 "+remoteLayoutDirectory); err != nil {
		return err
	}
	file, err := os.Open(".tmp/layout-clean-investigation-20261004/probe-linux")
	if err != nil {
		return err
	}
	defer file.Close()
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = file
	return session.Run("umask 077; cat > " + remoteLayoutDirectory + "/probe && chmod 700 " + remoteLayoutDirectory + "/probe")
}

func smallCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	data, err := session.CombinedOutput(command)
	return string(data), err
}

func invokeProbe(client *ssh.Client, call invocation) error {
	data, err := json.Marshal(call.Input)
	if err != nil {
		return err
	}
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = strings.NewReader(string(data))
	output, commandErr := session.CombinedOutput(call.Command)
	if err := os.WriteFile(filepath.Join(inventoryRoot, call.Name), output, 0600); err != nil {
		return err
	}
	if commandErr != nil {
		return fmt.Errorf("%s: %w: %s", call.Name, commandErr, output)
	}
	return nil
}
