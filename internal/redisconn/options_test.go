package redisconn

import "testing"

func TestOptionsKeepsPlainAddress(t *testing.T) {
	opts, err := Options("redis.local:6379", "secret", 2)
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	if opts.Addr != "redis.local:6379" || opts.Password != "secret" || opts.DB != 2 {
		t.Fatalf("unexpected options: addr=%q password=%q db=%d", opts.Addr, opts.Password, opts.DB)
	}
	if opts.TLSConfig != nil {
		t.Fatalf("plain redis address should not enable TLS")
	}
}

func TestOptionsParsesRedisURL(t *testing.T) {
	opts, err := Options("redis://:url-secret@redis.local:6379/3", "", 0)
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	if opts.Addr != "redis.local:6379" || opts.Password != "url-secret" || opts.DB != 3 {
		t.Fatalf("unexpected options: addr=%q password=%q db=%d", opts.Addr, opts.Password, opts.DB)
	}
	if opts.TLSConfig != nil {
		t.Fatalf("redis URL should not enable TLS")
	}
}

func TestOptionsParsesRedissURL(t *testing.T) {
	opts, err := Options("rediss://:url-secret@redis.local:6380/4", "", 0)
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	if opts.Addr != "redis.local:6380" || opts.Password != "url-secret" || opts.DB != 4 {
		t.Fatalf("unexpected options: addr=%q password=%q db=%d", opts.Addr, opts.Password, opts.DB)
	}
	if opts.TLSConfig == nil {
		t.Fatalf("rediss URL should enable TLS")
	}
}

func TestOptionsAllowsExternalOverridesForURL(t *testing.T) {
	opts, err := Options("rediss://:url-secret@redis.local:6380/4", "external-secret", 5)
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	if opts.Password != "external-secret" || opts.DB != 5 {
		t.Fatalf("expected external overrides, got password=%q db=%d", opts.Password, opts.DB)
	}
	if opts.TLSConfig == nil {
		t.Fatalf("rediss URL should keep TLS with external overrides")
	}
}

func TestOptionsRejectsInvalidURL(t *testing.T) {
	if _, err := Options("redis://:bad", "", 0); err == nil {
		t.Fatalf("expected invalid redis URL error")
	}
}

func TestWarnPlaintextCredentialsDoesNotBlock(t *testing.T) {
	opts, err := Options("redis://:secret@redis.local:6379/0", "", 0)
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	WarnPlaintextCredentials(nil, "test", opts)
}
