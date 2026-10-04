package providers

import (
	"context"

	"veloxmesh/internal/llm"
)

type protectedAdapter struct {
	ProviderAdapter
	policy ProtectionPolicy
}
type protectedStream struct {
	*protectedAdapter
	stream StreamAdapter
}
type protectedEmbed struct {
	*protectedAdapter
	embed EmbedAdapter
}
type protectedBoth struct {
	*protectedAdapter
	stream StreamAdapter
	embed  EmbedAdapter
}

func (adapter *protectedAdapter) unprotectedAdapter() ProviderAdapter { return adapter.ProviderAdapter }

func (adapter *protectedAdapter) Complete(ctx context.Context, request *llm.LLMRequest) (*llm.LLMResponse, error) {
	run, err := adapter.policy.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer run.finish()
	response, err := adapter.ProviderAdapter.Complete(run.ctx, request)
	if err := run.resultError(err); err != nil {
		return nil, err
	}
	run.contentReceived()
	return response, nil
}

func protectedEmbedding(ctx context.Context, adapter *protectedAdapter, call func(context.Context) (*llm.EmbeddingResponse, error)) (*llm.EmbeddingResponse, error) {
	run, err := adapter.policy.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer run.finish()
	response, err := call(run.ctx)
	if err := run.resultError(err); err != nil {
		return nil, err
	}
	run.contentReceived()
	return response, nil
}

func (adapter *protectedEmbed) Embed(ctx context.Context, request *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return protectedEmbedding(ctx, adapter.protectedAdapter, func(child context.Context) (*llm.EmbeddingResponse, error) {
		return adapter.embed.Embed(child, request)
	})
}

func (adapter *protectedBoth) Embed(ctx context.Context, request *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return protectedEmbedding(ctx, adapter.protectedAdapter, func(child context.Context) (*llm.EmbeddingResponse, error) {
		return adapter.embed.Embed(child, request)
	})
}

func (adapter *protectedStream) Stream(ctx context.Context, request *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	return adapter.protectedAdapter.startStream(ctx, request, adapter.stream.Stream)
}

func (adapter *protectedBoth) Stream(ctx context.Context, request *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	return adapter.protectedAdapter.startStream(ctx, request, adapter.stream.Stream)
}
