package pipeline

import (
	"context"

	"veloxmesh/internal/llm"
)

// RunState carries state (like PII mappings) between request and response processing.
type RunState struct {
	PIIMappings map[string]string
}

// RequestScope carries metadata about the request for logging and rule context.
type RequestScope struct {
	UserID    string
	RequestID string
}

// Handler handles a specific semantic rule on the request and/or response side.
type Handler interface {
	Name() RuleName
	// ProcessRequest modifies req and/or run state. Return ErrFilterBlock to intentionally block the request.
	ProcessRequest(ctx context.Context, scope RequestScope, state *RunState, req *llm.LLMRequest, config RuleConfig) error
	// ProcessResponse modifies resp and/or run state. Return ErrFilterBlock to intentionally block the response.
	ProcessResponse(ctx context.Context, scope RequestScope, state *RunState, resp *llm.LLMResponse, config RuleConfig) error
}


// Registry holds all available handlers.
type Registry struct {
	handlers map[RuleName]Handler
}

func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[RuleName]Handler),
	}
}

func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&FilterHandler{})
	r.Register(&PIIHandler{})
	r.Register(&RewriteHandler{})
	r.Register(&RTKHandler{})
	r.Register(&HeadroomHandler{})
	r.Register(&CavemanHandler{})
	r.Register(&PonytailHandler{})
	return r
}

func (r *Registry) Register(h Handler) {
	r.handlers[h.Name()] = h
}

func (r *Registry) Get(name RuleName) Handler {
	return r.handlers[name]
}
