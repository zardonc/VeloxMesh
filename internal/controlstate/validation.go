package controlstate

import (
	"fmt"
	"net/url"
)

func ValidateProviderMutation(m *ProviderMutation, isCreate bool) []FieldError {
	var errors []FieldError

	if m.ID == "" {
		errors = append(errors, FieldError{Field: "id", Code: "required", Message: "Provider ID is required"})
	}
	if m.Name == "" {
		errors = append(errors, FieldError{Field: "name", Code: "required", Message: "Provider Name is required"})
	}

	if m.Type != "openai-compatible" && m.Type != "anthropic" && m.Type != "gemini" {
		errors = append(errors, FieldError{Field: "type", Code: "unsupported_provider_type", Message: fmt.Sprintf("Unsupported provider type: %s", m.Type)})
	}

	if err := ValidateProviderBaseURL(m.BaseURL); err != nil {
		errors = append(errors, FieldError{Field: "base_url", Code: "invalid_url", Message: err.Error()})
	}

	if isCreate && (m.APIKey == nil || *m.APIKey == "") {
		errors = append(errors, FieldError{Field: "api_key", Code: "secret_required", Message: "API Key is required on create"})
	}

	if errs := ValidateProviderModels(m.Models, m.DefaultModel); len(errs) > 0 {
		errors = append(errors, errs...)
	}

	return errors
}

func ValidateProviderBaseURL(baseURL string) error {
	if baseURL == "" {
		return fmt.Errorf("Base URL is required")
	}
	u, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return fmt.Errorf("Invalid base URL format")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("Base URL must use http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("Base URL host cannot be empty")
	}
	return nil
}

func ValidateProviderModels(models []string, defaultModel *string) []FieldError {
	var errors []FieldError

	if defaultModel != nil && *defaultModel != "" {
		found := false
		for _, m := range models {
			if m == *defaultModel {
				found = true
				break
			}
		}
		if !found {
			errors = append(errors, FieldError{
				Field:   "default_model",
				Code:    "default_model_not_in_models",
				Message: fmt.Sprintf("Default model %q not found in provided models", *defaultModel),
			})
		}
	}

	return errors
}

func ValidateRoutingConfig(rc *RoutingConfig, providers []*ProviderRecord, backend string, redisConfigured bool) []FieldError {
	var errs []FieldError

	if rc == nil {
		errs = append(errs, FieldError{Field: "routing_config", Code: "required", Message: "routing config is nil"})
		return errs
	}

	if rc.Strategy != "round-robin" && rc.Strategy != "least-latency" && rc.Strategy != "priority" {
		errs = append(errs, FieldError{Field: "strategy", Code: "invalid_strategy", Message: fmt.Sprintf("invalid strategy: %s", rc.Strategy)})
	}

	activeCount := 0
	defaultFound := false
	for _, p := range providers {
		if !p.Enabled {
			continue
		}
		activeCount++
		if p.ID == rc.DefaultProvider {
			defaultFound = true
		}
	}

	if rc.DefaultProvider != "" && !defaultFound {
		errs = append(errs, FieldError{Field: "default_provider", Code: "not_found", Message: fmt.Sprintf("default_provider '%s' not found or inactive", rc.DefaultProvider)})
	}

	if !rc.FallbackEnabled {
		if rc.MaxAttempts != 1 {
			errs = append(errs, FieldError{Field: "max_attempts", Code: "invalid_max_attempts", Message: "max_attempts must be 1 when fallback is disabled"})
		}
	} else {
		if rc.MaxAttempts < 1 {
			errs = append(errs, FieldError{Field: "max_attempts", Code: "invalid_max_attempts", Message: "max_attempts must be at least 1 when fallback is enabled"})
		}
		if rc.MaxAttempts > activeCount && activeCount > 0 {
			errs = append(errs, FieldError{Field: "max_attempts", Code: "invalid_max_attempts", Message: fmt.Sprintf("max_attempts (%d) cannot exceed active eligible provider count (%d)", rc.MaxAttempts, activeCount)})
		}
	}

	if backend == "sqlite" {
		// Valid
	} else if backend == "postgres" {
		if !redisConfigured {
			errs = append(errs, FieldError{Field: "mode", Code: "invalid_mode", Message: "full-mode distributed capability requires Redis when PostgreSQL is used"})
		}
	}

	return errs
}

func ValidateComboMutation(m *ComboMutation, activeModels []string) []FieldError {
	var errs []FieldError

	if m.ID == "" {
		errs = append(errs, FieldError{Field: "id", Code: "required", Message: "Combo ID is required"})
	}
	if m.Name == "" {
		errs = append(errs, FieldError{Field: "name", Code: "required", Message: "Combo Name is required"})
	}

	if m.Strategy != "round-robin" && m.Strategy != "fusion" && m.Strategy != "capacity-auto-switch" {
		errs = append(errs, FieldError{Field: "strategy", Code: "invalid_strategy", Message: fmt.Sprintf("Unsupported strategy: %s", m.Strategy)})
	}

	if len(m.Members) == 0 {
		errs = append(errs, FieldError{Field: "members", Code: "required", Message: "Combo must have at least one member"})
	}

	if m.Strategy == "fusion" {
		if m.Judge == nil || *m.Judge == "" {
			errs = append(errs, FieldError{Field: "judge", Code: "required_for_fusion", Message: "Judge model is required for fusion strategy"})
		}
	}

	validModels := make(map[string]bool)
	for _, pm := range activeModels {
		validModels[pm] = true
	}

	if validModels[m.Name] {
		errs = append(errs, FieldError{Field: "name", Code: "conflict", Message: fmt.Sprintf("Combo name %q conflicts with an active provider model", m.Name)})
	}

	for _, member := range m.Members {
		if !validModels[member] {
			errs = append(errs, FieldError{Field: "members", Code: "invalid_model", Message: fmt.Sprintf("Member model %q is not a currently connected provider model", member)})
		}
	}

	if m.Judge != nil && *m.Judge != "" {
		if !validModels[*m.Judge] {
			errs = append(errs, FieldError{Field: "judge", Code: "invalid_model", Message: fmt.Sprintf("Judge model %q is not a currently connected provider model", *m.Judge)})
		}
	}

	return errs
}
