package main

import (
	"bufio"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"veloxmesh-ai-gateway-dashboard/internal/bff"
)

func main() {
	config, err := loadConfig(".env2.local")
	if err != nil {
		log.Printf("using defaults because config could not be loaded: %v", err)
	}

	addr := firstNonEmpty(os.Getenv("BFF_ADDR"), ":8080")
	log.Printf("gateway/BFF listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, bff.NewServer(config)); err != nil {
		log.Fatal(err)
	}
}

func loadConfig(path string) (bff.Config, error) {
	values, err := readEnvFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return bff.Config{}, err
		}
		values = map[string]string{}
	}

	return bff.Config{
		DevAPIKey:                 values["DEV_API_KEY"],
		ProviderName:              values["DEFAULT_PROVIDER"],
		BaseURL:                   values["SANS_BASE_URL"],
		DefaultModel:              values["SANS_PRIMARY_DEFAULT_MODEL"],
		Models:                    splitCSV(values["SANS_PRIMARY_MODELS"]),
		StatePath:                 firstNonEmpty(os.Getenv("ADMIN_STATE_PATH"), values["ADMIN_STATE_PATH"], "tmp/admin-state.json"),
		RedisAddr:                 firstNonEmpty(os.Getenv("REDIS_ADDR"), values["REDIS_ADDR"], "127.0.0.1:6379"),
		QdrantURL:                 firstNonEmpty(os.Getenv("QDRANT_URL"), values["QDRANT_URL"], "http://127.0.0.1:6333"),
		QdrantAPIKey:              firstNonEmpty(os.Getenv("QDRANT_API_KEY"), values["QDRANT_API_KEY"]),
		QdrantBenchmarkCollection: firstNonEmpty(values["QDRANT_BENCHMARK_COLLECTION"], "veloxmesh_benchmarks"),
		EmailOutboxPath:           firstNonEmpty(os.Getenv("EMAIL_OUTBOX_PATH"), values["EMAIL_OUTBOX_PATH"], "tmp/email-outbox.log"),
		SMTPHost:                  values["SMTP_HOST"],
		SMTPPort:                  firstNonEmpty(values["SMTP_PORT"], "587"),
		SMTPUsername:              values["SMTP_USERNAME"],
		SMTPPassword:              values["SMTP_PASSWORD"],
		SMTPFrom:                  values["SMTP_FROM"],
		DemoMode:                  strings.EqualFold(values["DASHBOARD_DEMO_MODE"], "true"),
		BootstrapAdminEmail:       firstNonEmpty(os.Getenv("ADMIN_BOOTSTRAP_EMAIL"), values["ADMIN_BOOTSTRAP_EMAIL"]),
		BootstrapAdminUsername:    firstNonEmpty(os.Getenv("ADMIN_BOOTSTRAP_USERNAME"), values["ADMIN_BOOTSTRAP_USERNAME"]),
		BootstrapAdminPassword:    firstNonEmpty(os.Getenv("ADMIN_BOOTSTRAP_PASSWORD"), values["ADMIN_BOOTSTRAP_PASSWORD"]),
	}, nil
}

func readEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, errors.New("invalid env line: " + line)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values, scanner.Err()
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
