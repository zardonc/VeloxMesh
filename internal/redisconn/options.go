package redisconn

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/redis/go-redis/v9"
)

func Options(addr, password string, db int) (*redis.Options, error) {
	if strings.Contains(addr, "://") {
		opts, err := redis.ParseURL(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid redis addr: %w", err)
		}
		if password != "" {
			opts.Password = password
		}
		if db != 0 {
			opts.DB = db
		}
		return opts, nil
	}
	return &redis.Options{Addr: addr, Password: password, DB: db}, nil
}

func WarnPlaintextCredentials(logger *slog.Logger, component string, opts *redis.Options) {
	if opts == nil || opts.Password == "" || opts.TLSConfig != nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("redis password configured without TLS; connection will proceed over plaintext", "component", component, "addr", opts.Addr)
}
