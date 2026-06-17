# Provider Adapter Conformance Harness

This package provides a shared test harness for any `providers.ProviderAdapter` implementation.

## Future Adapter Contract

To implement and register a new provider adapter, you must ensure it passes this conformance harness. 
The harness guarantees provider-neutral behavior, safe error mapping, and proper request/response handling.

### Authoring Guide

1. **Create Fake Upstream Behavior:** Set up an `httptest.Server` or a mock SDK client in your provider's `*_test.go` file. Ensure it returns deterministic responses and errors. **No live network calls or credentials are allowed.**
2. **Instantiate ConformanceSpec:** Create a `ConformanceSpec` configuring all expected behaviors for your adapter.
3. **RunConformance:** Call `adaptertest.RunConformance(t, spec)` in a test named something like `TestAdapter_Conformance`.
4. **Local SDK/Native Tests:** Keep provider-specific transport, SDK behavior, or edge case tests in your adapter's test file. The harness does not replace them.
5. **Production Isolation:** Do not import `adaptertest` in production code.

### Example

```go
func TestAdapter_Conformance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock logic based on some state
	}))
	defer server.Close()

	adapter := NewAdapter(Config{BaseURL: server.URL})

	spec := adaptertest.ConformanceSpec{
		Adapter: adapter,
		ExpectedID: "my-provider",
		// ... populate spec ...
	}

	adaptertest.RunConformance(t, spec)
}
```
