package controlstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	gwErr "veloxmesh/internal/errors"
)

func RequestFingerprint(method, path string, body []byte) string {
	var j map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &j); err == nil {
			if _, ok := j["api_key"]; ok {
				j["api_key"] = "***REDACTED***"
			}
			body, _ = json.Marshal(j)
		}
	}

	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte("|"))
	h.Write([]byte(path))
	h.Write([]byte("|"))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func IdempotencyKeyFromRequest(r *http.Request) string {
	return r.Header.Get("Idempotency-Key")
}

// IdempotencyResult wraps a serialized response that was replayed from the database.
type IdempotencyResult struct {
	Response []byte
	Status   int
}

// WithIdempotency provides idempotency key support for admin mutations.
func (s *AdminProviderService) WithIdempotency(
	ctx context.Context,
	key string,
	actionName string,
	method string,
	path string,
	reqBody []byte,
	execute func(context.Context) (interface{}, error),
) (interface{}, error) {
	if key == "" {
		return execute(ctx)
	}

	fingerprint := RequestFingerprint(method, path, reqBody)

	// Check existing
	existing, err := s.repo.Idempotency().Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to check idempotency key: %w", err)
	}

	if existing != nil {
		if existing.Fingerprint != fingerprint {
			return nil, gwErr.NewGatewayError("idempotency_key_conflict", "idempotency key conflict: payload mismatch", http.StatusConflict)
		}

		status, _ := strconv.Atoi(existing.Status)
		if status == 0 {
			status = http.StatusOK
		}

		return &IdempotencyResult{
			Response: []byte(existing.Response),
			Status:   status,
		}, nil
	}

	// Execute action
	result, err := execute(ctx)

	// We only record idempotency for successful calls and expected business errors (GatewayError/ValidationError)
	// Internal errors shouldn't be cached as idempotent usually, but let's cache what we can to be safe and avoid double-charging.
	// Actually, wait, it's best to always cache if an operation was attempted to prevent retry on side-effect causing operations.
	// But since the plan says "Store action name, key, fingerprint, response status, response body", let's cache the outcome.

	record := &IdempotencyRecord{
		Key:         key,
		ActionName:  actionName,
		Fingerprint: fingerprint,
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   time.Now().UTC().Add(24 * time.Hour), // 24h retention
	}

	if err != nil {
		// Attempt to extract known error status
		status := http.StatusInternalServerError
		if gwE, ok := err.(*gwErr.GatewayError); ok {
			status = gwE.HTTPStatus
		} else if IsValidationError(err) {
			status = http.StatusBadRequest
		}

		// Marshal error
		var errBody []byte
		if IsValidationError(err) {
			errBody, _ = json.Marshal(err)
		} else if gwE, ok := err.(*gwErr.GatewayError); ok {
			errBody, _ = json.Marshal(gwE)
		} else {
			errBody, _ = json.Marshal(map[string]string{
				"code":    "internal_error",
				"message": err.Error(),
			})
		}

		record.Status = strconv.Itoa(status)
		record.Response = string(errBody)

		_ = s.repo.Idempotency().Save(ctx, record)
		return nil, err
	}

	// Success case
	status := http.StatusOK
	if actionName == "provider.create" {
		status = http.StatusCreated
	} else if actionName == "provider.disable" || actionName == "provider.delete" {
		status = http.StatusNoContent
	}

	var resBody []byte
	if result != nil {
		resBody, _ = json.Marshal(result)
	}

	record.Status = strconv.Itoa(status)
	record.Response = string(resBody)
	_ = s.repo.Idempotency().Save(ctx, record)

	return result, nil
}
