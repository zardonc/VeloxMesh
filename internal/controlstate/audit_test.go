package controlstate

import (
	"strings"
	"testing"
)

func TestSafeAuditMetadata(t *testing.T) {
	input := map[string]interface{}{
		"provider_type": "openai-compatible",
		"api_key":       "sk-1234",
		"nonce":         "abc",
		"timeout":       "30s",
	}

	b := SafeAuditMetadata(input)
	str := string(b)

	if strings.Contains(str, "sk-1234") {
		t.Errorf("expected api_key to be redacted")
	}
	if strings.Contains(str, "abc") {
		t.Errorf("expected nonce to be redacted")
	}
	if !strings.Contains(str, "***REDACTED***") {
		t.Errorf("expected redacted string")
	}
	if !strings.Contains(str, "openai-compatible") {
		t.Errorf("expected safe data to be retained")
	}
}

// Full coverage would include testing PurgeAuditBefore etc, which is a simple passthrough.
