package main

import (
	"bufio"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"veloxmesh-ai-gateway-dashboard/internal/bff"
)

func main() {
	config, err := loadConfig(".env2.local")
	if err != nil {
		log.Fatal(err)
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

	config := bff.Config{
		DevAPIKey:                    values["DEV_API_KEY"],
		ProviderName:                 values["DEFAULT_PROVIDER"],
		BaseURL:                      values["SANS_BASE_URL"],
		DefaultModel:                 values["SANS_PRIMARY_DEFAULT_MODEL"],
		Models:                       splitCSV(values["SANS_PRIMARY_MODELS"]),
		StatePath:                    firstNonEmpty(os.Getenv("ADMIN_STATE_PATH"), values["ADMIN_STATE_PATH"], "tmp/admin-state.json"),
		RedisAddr:                    firstNonEmpty(os.Getenv("REDIS_ADDR"), values["REDIS_ADDR"], "127.0.0.1:6379"),
		QdrantURL:                    firstNonEmpty(os.Getenv("QDRANT_URL"), values["QDRANT_URL"], "http://127.0.0.1:6333"),
		QdrantAPIKey:                 firstNonEmpty(os.Getenv("QDRANT_API_KEY"), values["QDRANT_API_KEY"]),
		QdrantBenchmarkCollection:    firstNonEmpty(values["QDRANT_BENCHMARK_COLLECTION"], "veloxmesh_benchmarks"),
		EmailOutboxPath:              firstNonEmpty(os.Getenv("EMAIL_OUTBOX_PATH"), values["EMAIL_OUTBOX_PATH"], "tmp/email-outbox.log"),
		SMTPHost:                     firstNonEmpty(os.Getenv("SMTP_HOST"), values["SMTP_HOST"]),
		SMTPPort:                     firstNonEmpty(os.Getenv("SMTP_PORT"), values["SMTP_PORT"], "587"),
		SMTPUsername:                 firstNonEmpty(os.Getenv("SMTP_USERNAME"), values["SMTP_USERNAME"]),
		SMTPPasswordFile:             firstNonEmpty(os.Getenv("SMTP_PASSWORD_FILE"), values["SMTP_PASSWORD_FILE"]),
		SMTPFrom:                     firstNonEmpty(os.Getenv("SMTP_FROM"), values["SMTP_FROM"]),
		SMTPTLSMode:                  firstNonEmpty(os.Getenv("SMTP_TLS_MODE"), values["SMTP_TLS_MODE"], "starttls"),
		SMTPServerName:               firstNonEmpty(os.Getenv("SMTP_SERVER_NAME"), values["SMTP_SERVER_NAME"]),
		SMTPTimeout:                  parseDuration(firstNonEmpty(os.Getenv("SMTP_TIMEOUT"), values["SMTP_TIMEOUT"]), 10*time.Second),
		DemoMode:                     strings.EqualFold(firstNonEmpty(os.Getenv("DASHBOARD_DEMO_MODE"), values["DASHBOARD_DEMO_MODE"], "false"), "true"),
		TestMode:                     strings.EqualFold(firstNonEmpty(os.Getenv("DASHBOARD_TEST_MODE"), "false"), "true"),
		SessionTTL:                   parseDuration(firstNonEmpty(os.Getenv("SESSION_TTL"), values["SESSION_TTL"]), 8*time.Hour),
		SessionCookieSecure:          strings.EqualFold(firstNonEmpty(os.Getenv("SESSION_COOKIE_SECURE"), values["SESSION_COOKIE_SECURE"], "false"), "true"),
		VerificationSendEmailLimit:   parsePositiveInt(firstNonEmpty(os.Getenv("VERIFICATION_SEND_EMAIL_LIMIT"), values["VERIFICATION_SEND_EMAIL_LIMIT"]), 3),
		VerificationSendIPLimit:      parsePositiveInt(firstNonEmpty(os.Getenv("VERIFICATION_SEND_IP_LIMIT"), values["VERIFICATION_SEND_IP_LIMIT"]), 20),
		VerificationVerifyEmailLimit: parsePositiveInt(firstNonEmpty(os.Getenv("VERIFICATION_VERIFY_EMAIL_LIMIT"), values["VERIFICATION_VERIFY_EMAIL_LIMIT"]), 10),
		VerificationVerifyIPLimit:    parsePositiveInt(firstNonEmpty(os.Getenv("VERIFICATION_VERIFY_IP_LIMIT"), values["VERIFICATION_VERIFY_IP_LIMIT"]), 50),
		VerificationRateWindow:       parseDuration(firstNonEmpty(os.Getenv("VERIFICATION_RATE_WINDOW"), values["VERIFICATION_RATE_WINDOW"]), 15*time.Minute),
		GatewayAdminURL:              firstNonEmpty(os.Getenv("VELOXMESH_ADMIN_URL"), values["VELOXMESH_ADMIN_URL"]),
		GatewayDataURL:               firstNonEmpty(os.Getenv("VELOXMESH_DATA_URL"), values["VELOXMESH_DATA_URL"]),
		GatewayMetricsURL:            firstNonEmpty(os.Getenv("VELOXMESH_METRICS_URL"), values["VELOXMESH_METRICS_URL"]),
		GatewayAdminAPIKey:           firstNonEmpty(os.Getenv("VELOXMESH_ADMIN_API_KEY"), values["VELOXMESH_ADMIN_API_KEY"]),
		GatewayDataAPIKey:            firstNonEmpty(os.Getenv("VELOXMESH_DATA_API_KEY"), values["VELOXMESH_DATA_API_KEY"]),
		GatewayAPITimeout:            parseDuration(firstNonEmpty(os.Getenv("VELOXMESH_API_TIMEOUT"), values["VELOXMESH_API_TIMEOUT"]), 10*time.Second),
		BootstrapAdminEmail:          firstNonEmpty(os.Getenv("ADMIN_BOOTSTRAP_EMAIL"), values["ADMIN_BOOTSTRAP_EMAIL"]),
		BootstrapAdminUsername:       firstNonEmpty(os.Getenv("ADMIN_BOOTSTRAP_USERNAME"), values["ADMIN_BOOTSTRAP_USERNAME"]),
		BootstrapAdminPassword:       firstNonEmpty(os.Getenv("ADMIN_BOOTSTRAP_PASSWORD"), values["ADMIN_BOOTSTRAP_PASSWORD"]),
	}
	password, err := smtpPassword(config.SMTPPasswordFile, os.Getenv("SMTP_PASSWORD"))
	if err != nil {
		return bff.Config{}, err
	}
	config.SMTPPassword = password
	if err := validateSMTPConfig(config); err != nil {
		return bff.Config{}, err
	}
	return config, nil
}

func smtpPassword(secretFile string, environmentValue string) (string, error) {
	if strings.TrimSpace(secretFile) == "" {
		return environmentValue, nil
	}
	data, err := os.ReadFile(strings.TrimSpace(secretFile))
	if err != nil {
		return "", errors.New("SMTP password file could not be read")
	}
	password := strings.TrimSpace(string(data))
	if password == "" {
		return "", errors.New("SMTP password file is empty")
	}
	return password, nil
}

func validateSMTPConfig(config bff.Config) error {
	configured := strings.TrimSpace(config.SMTPHost) != "" || strings.TrimSpace(config.SMTPUsername) != "" ||
		strings.TrimSpace(config.SMTPPassword) != "" || strings.TrimSpace(config.SMTPFrom) != "" || strings.TrimSpace(config.SMTPPasswordFile) != ""
	if !configured {
		return nil
	}
	if strings.TrimSpace(config.SMTPHost) == "" || strings.TrimSpace(config.SMTPUsername) == "" || strings.TrimSpace(config.SMTPPassword) == "" || strings.TrimSpace(config.SMTPFrom) == "" {
		return errors.New("SMTP configuration is incomplete")
	}
	mode := strings.ToLower(strings.TrimSpace(config.SMTPTLSMode))
	if mode != "starttls" && mode != "tls" {
		return errors.New("SMTP TLS mode must be starttls or tls")
	}
	return nil
}

func parseDuration(value string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
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
