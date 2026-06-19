package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"veloxmesh/internal/config"
)

func TestAdminAuth(t *testing.T) {
	cfg := &config.Config{
		AdminAPIKey: "secret-admin-key",
		DevAPIKey:   "dev-key-should-fail",
	}

	handler := AdminAuth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	tests := []struct {
		name           string
		authHeader     string
		cfgAdminKey    string // override admin key for specific test
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "Valid Admin Key",
			authHeader:     "Bearer secret-admin-key",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Missing Header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "admin_missing_authorization",
		},
		{
			name:           "Malformed Header",
			authHeader:     "secret-admin-key",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "admin_invalid_authorization",
		},
		{
			name:           "Data Plane Dev Key Fails",
			authHeader:     "Bearer dev-key-should-fail",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "admin_invalid_api_key",
		},
		{
			name:           "Wrong Admin Key",
			authHeader:     "Bearer wrong-key",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "admin_invalid_api_key",
		},
		{
			name:           "Unconfigured Admin Key Fails Even With Dev Key",
			authHeader:     "Bearer dev-key-should-fail",
			cfgAdminKey:    "", // Simulate admin key not configured
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "admin_invalid_api_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/admin/v1/providers", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			// temporary override of admin key if specified
			originalKey := cfg.AdminAPIKey
			if tt.cfgAdminKey == "" && tt.name == "Unconfigured Admin Key Fails Even With Dev Key" {
				cfg.AdminAPIKey = ""
			}
			defer func() { cfg.AdminAPIKey = originalKey }()

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, status)
			}

			if tt.expectedStatus != http.StatusOK {
				var errResp map[string]interface{}
				if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if code, ok := errResp["code"].(string); !ok || code != tt.expectedCode {
					t.Errorf("expected error code %s, got %v", tt.expectedCode, errResp["code"])
				}
			}
		})
	}
}
