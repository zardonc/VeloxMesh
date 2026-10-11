package gemini

import (
	"context"
	"math"
	"net/http"
	"strings"

	"google.golang.org/genai"

	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

func (a *Adapter) Embed(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	if req == nil || len(req.Input) == 0 {
		return nil, gatewayErr.NewGatewayError(gatewayErr.InvalidRequest, "embedding input is required", http.StatusBadRequest)
	}
	contents := make([]*genai.Content, len(req.Input))
	for index, input := range req.Input {
		if strings.TrimSpace(input) == "" {
			return nil, gatewayErr.NewGatewayError(gatewayErr.InvalidRequest, "embedding input must not be blank", http.StatusBadRequest)
		}
		contents[index] = genai.NewContentFromText(input, genai.RoleUser)
	}
	model := a.requestModel(req.Model)
	response, err := a.client.Models.EmbedContent(ctx, strings.TrimPrefix(model, "gemini/"), contents, nil)
	if err != nil {
		return nil, a.mapError(err)
	}
	if response == nil || len(response.Embeddings) != len(req.Input) {
		return nil, invalidEmbeddingResponse()
	}
	data := make([]llm.Embedding, len(response.Embeddings))
	for index, embedding := range response.Embeddings {
		if embedding == nil || !validEmbeddingVector(embedding.Values) {
			return nil, invalidEmbeddingResponse()
		}
		data[index] = llm.Embedding{Index: index, Embedding: append([]float32(nil), embedding.Values...)}
	}
	return &llm.EmbeddingResponse{Model: model, Data: data}, nil
}

func validEmbeddingVector(values []float32) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return true
}

func invalidEmbeddingResponse() error {
	return gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "invalid embedding response", http.StatusBadGateway)
}
