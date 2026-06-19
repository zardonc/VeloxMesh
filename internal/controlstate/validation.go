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
