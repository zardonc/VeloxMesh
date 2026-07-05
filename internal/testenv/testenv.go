package testenv

import (
	"net/url"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

const defaultPostgresPort = "5432"
const defaultQdrantPort = "6334"

func Load() {
	root, ok := moduleRoot()
	if !ok {
		return
	}
	_ = godotenv.Load(filepath.Join(root, ".env.local"), filepath.Join(root, ".env"))
	setPostgresTestDSN()
	setQdrantAddr()
}

func moduleRoot() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func setPostgresTestDSN() {
	if os.Getenv("POSTGRES_TEST_DSN") != "" {
		return
	}
	host := os.Getenv("DEV_SERVER_IP")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	db := os.Getenv("POSTGRES_DB")
	if host == "" || user == "" || password == "" || db == "" {
		return
	}
	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = defaultPostgresPort
	}
	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     host + ":" + port,
		Path:     "/" + db,
		RawQuery: "sslmode=disable",
	}
	_ = os.Setenv("POSTGRES_TEST_DSN", dsn.String())
}

func setQdrantAddr() {
	if os.Getenv("QDRANT_ADDR") != "" {
		return
	}
	host := os.Getenv("DEV_SERVER_IP")
	if host == "" {
		return
	}
	port := os.Getenv("QDRANT_PORT")
	if port == "" {
		port = defaultQdrantPort
	}
	_ = os.Setenv("QDRANT_ADDR", host+":"+port)
}
