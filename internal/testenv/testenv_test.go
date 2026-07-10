package testenv

import (
	"os"
	"testing"
)

func TestSetQdrantAddrFromProjectEnv(t *testing.T) {
	t.Setenv("QDRANT_ADDR", "")
	t.Setenv("DEV_SERVER_IP", "10.0.0.7")
	t.Setenv("QDRANT_PORT", "6334")

	setQdrantAddr()

	if got := os.Getenv("QDRANT_ADDR"); got != "10.0.0.7:6334" {
		t.Fatalf("expected qdrant addr from project env, got %q", got)
	}
}

func TestSetQdrantAddrPreservesExplicitValue(t *testing.T) {
	t.Setenv("QDRANT_ADDR", "explicit:6334")
	t.Setenv("DEV_SERVER_IP", "10.0.0.7")

	setQdrantAddr()

	if got := os.Getenv("QDRANT_ADDR"); got != "explicit:6334" {
		t.Fatalf("expected explicit qdrant addr, got %q", got)
	}
}

func TestSetRedisAddrFromProjectEnv(t *testing.T) {
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("DEV_SERVER_IP", "10.0.0.7")
	t.Setenv("REDIS_PORT", "6379")

	setRedisAddr()

	if got := os.Getenv("REDIS_ADDR"); got != "10.0.0.7:6379" {
		t.Fatalf("expected redis addr from project env, got %q", got)
	}
}

func TestSetRedisAddrPreservesExplicitValue(t *testing.T) {
	t.Setenv("REDIS_ADDR", "explicit:6379")
	t.Setenv("DEV_SERVER_IP", "10.0.0.7")

	setRedisAddr()

	if got := os.Getenv("REDIS_ADDR"); got != "explicit:6379" {
		t.Fatalf("expected explicit redis addr, got %q", got)
	}
}
