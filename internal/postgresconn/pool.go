package postgresconn

import (
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

func PoolConfig(dsn string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid postgres dsn: %w", err)
	}
	return cfg, nil
}

func WarnPlaintextCredentials(logger *slog.Logger, component string, cfg *pgxpool.Config) {
	if cfg == nil || cfg.ConnConfig == nil || cfg.ConnConfig.Password == "" || cfg.ConnConfig.TLSConfig != nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("postgres password configured without TLS; connection will proceed over plaintext", "component", component, "host", cfg.ConnConfig.Host, "database", cfg.ConnConfig.Database)
}
