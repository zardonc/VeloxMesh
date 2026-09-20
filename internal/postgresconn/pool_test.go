package postgresconn

import "testing"

func TestPoolConfigUsesDSNSSLMode(t *testing.T) {
	tlsConfig, err := PoolConfig("postgres://user:pass@localhost:5432/db?sslmode=require")
	if err != nil {
		t.Fatalf("parse TLS postgres DSN: %v", err)
	}
	if tlsConfig.ConnConfig.TLSConfig == nil {
		t.Fatalf("sslmode=require should enable TLS in pgx config")
	}

	plainConfig, err := PoolConfig("postgres://user:pass@localhost:5432/db?sslmode=disable")
	if err != nil {
		t.Fatalf("parse plaintext postgres DSN: %v", err)
	}
	if plainConfig.ConnConfig.TLSConfig != nil {
		t.Fatalf("sslmode=disable should leave TLS disabled in pgx config")
	}
}

func TestWarnPlaintextCredentialsDoesNotBlock(t *testing.T) {
	cfg, err := PoolConfig("postgres://user:pass@localhost:5432/db?sslmode=disable")
	if err != nil {
		t.Fatalf("parse postgres DSN: %v", err)
	}
	WarnPlaintextCredentials(nil, "test", cfg)
}

func TestPoolConfigRejectsInvalidDSN(t *testing.T) {
	if _, err := PoolConfig("postgres://%zz"); err == nil {
		t.Fatalf("expected invalid postgres DSN error")
	}
}
