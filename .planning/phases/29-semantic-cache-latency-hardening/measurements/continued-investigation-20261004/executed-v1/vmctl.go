package main

import (
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

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("script and evidence path required")
	}
	env, err := godotenv.Read(".env.local")
	if err != nil {
		return err
	}
	userDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	callback, err := knownhosts.New(filepath.Join(userDir, ".ssh", "known_hosts"))
	if err != nil {
		return err
	}
	port := env["DEV_SERVER_PORT"]
	if port == "" {
		port = "22"
	}
	client, err := ssh.Dial("tcp", net.JoinHostPort(env["DEV_SERVER_IP"], port), &ssh.ClientConfig{
		User: env["DEV_SERVER_USER"], Auth: []ssh.AuthMethod{ssh.Password(env["DEV_SERVER_PW"])},
		HostKeyCallback: callback, Timeout: 10 * time.Second})
	if err != nil {
		return err
	}
	defer client.Close()
	timer := time.AfterFunc(60*time.Second, func() { client.Close() })
	defer timer.Stop()
	script, err := os.Open(os.Args[1])
	if err != nil {
		return err
	}
	defer script.Close()
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = script
	output, runErr := session.CombinedOutput("timeout 55s python3 -")
	redacted := redact(string(output), env)
	if err := os.WriteFile(os.Args[2], []byte(redacted), 0600); err != nil {
		return err
	}
	fmt.Print(redacted)
	return runErr
}

func redact(output string, env map[string]string) string {
	for key, value := range env {
		key = strings.ToUpper(key)
		if len(value) >= 4 && (strings.Contains(key, "KEY") || strings.Contains(key, "PW") ||
			strings.Contains(key, "TOKEN") || strings.Contains(key, "SECRET") || strings.Contains(key, "PASSWORD")) {
			output = strings.ReplaceAll(output, value, "[REDACTED]")
		}
	}
	return output
}
