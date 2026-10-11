package providers

import "veloxmesh/internal/llm"

// ProviderType represents the type of the underlying provider adapter.
type ProviderType string

const (
	ProviderTypeOpenAICompatible ProviderType = "openai-compatible"
	ProviderTypeAnthropic        ProviderType = "anthropic"
	ProviderTypeGemini           ProviderType = "gemini"
)

// Operation represents a supported operation.
type Operation string

const (
	OperationChatCompletions Operation = "chat_completions"
	OperationEmbeddings      Operation = "embeddings"
)

// Modality represents an input or output modality.
type Modality string

const (
	ModalityText  Modality = "text"
	ModalityImage Modality = "image"
	ModalityPDF   Modality = "pdf"
	ModalityAudio Modality = "audio"
)

// GenerationParameter represents a supported parameter for generation.
type GenerationParameter string

const (
	GenerationParameterTemperature GenerationParameter = "temperature"
	GenerationParameterMaxTokens   GenerationParameter = "max_tokens"
)

// ToolChoiceCapabilityMode identifies a normalized tool_choice mode.
type ToolChoiceCapabilityMode string

const (
	ToolChoiceCapabilityOmitted  ToolChoiceCapabilityMode = "omitted"
	ToolChoiceCapabilityAuto     ToolChoiceCapabilityMode = "auto"
	ToolChoiceCapabilityNone     ToolChoiceCapabilityMode = "none"
	ToolChoiceCapabilityRequired ToolChoiceCapabilityMode = "required"
	ToolChoiceCapabilityNamed    ToolChoiceCapabilityMode = "named"
)

// ToolProtocolCapability describes the tool protocol behavior a model can honor.
type ToolProtocolCapability struct {
	Definitions        bool
	AssistantToolCalls bool
	ToolResults        bool
	StreamingDeltas    bool
	ChoiceModes        map[ToolChoiceCapabilityMode]bool
}

// Clone returns an independent tool protocol capability snapshot.
func (c ToolProtocolCapability) Clone() ToolProtocolCapability {
	clone := ToolProtocolCapability{
		Definitions:        c.Definitions,
		AssistantToolCalls: c.AssistantToolCalls,
		ToolResults:        c.ToolResults,
		StreamingDeltas:    c.StreamingDeltas,
	}
	if c.ChoiceModes != nil {
		clone.ChoiceModes = make(map[ToolChoiceCapabilityMode]bool, len(c.ChoiceModes))
		for mode, supported := range c.ChoiceModes {
			clone.ChoiceModes[mode] = supported
		}
	}
	return clone
}

// Supports reports whether this model can honor normalized tool protocol requirements.
func (c ToolProtocolCapability) Supports(requirements llm.ToolProtocolRequirements, stream bool) bool {
	if !requirements.UsesProtocol() {
		return true
	}
	if !c.Definitions || !c.ChoiceModes[toolChoiceCapabilityMode(requirements)] {
		return false
	}
	if requirements.HasAssistantToolCall && !c.AssistantToolCalls {
		return false
	}
	if requirements.HasToolResult && !c.ToolResults {
		return false
	}
	return !stream || c.StreamingDeltas
}

func toolChoiceCapabilityMode(requirements llm.ToolProtocolRequirements) ToolChoiceCapabilityMode {
	if !requirements.HasExplicitChoice {
		return ToolChoiceCapabilityOmitted
	}
	switch requirements.ChoiceMode {
	case llm.ToolChoiceAuto:
		return ToolChoiceCapabilityAuto
	case llm.ToolChoiceNone:
		return ToolChoiceCapabilityNone
	case llm.ToolChoiceRequired:
		return ToolChoiceCapabilityRequired
	case llm.ToolChoiceNamed:
		return ToolChoiceCapabilityNamed
	default:
		return ""
	}
}

// CapabilitySet describes the supported capabilities of a provider adapter.
type CapabilitySet struct {
	ProviderType         ProviderType
	SupportedOperations  []Operation
	InputModalities      []Modality
	OutputModalities     []Modality
	Streaming            bool
	ToolCalling          bool
	ToolProtocol         ToolProtocolCapability
	GenerationParameters []GenerationParameter
}

// Clone returns a deep copy of the CapabilitySet.
func (c CapabilitySet) Clone() CapabilitySet {
	clone := CapabilitySet{
		ProviderType: c.ProviderType,
		Streaming:    c.Streaming,
		ToolCalling:  c.ToolCalling,
		ToolProtocol: c.ToolProtocol.Clone(),
	}

	if c.SupportedOperations != nil {
		clone.SupportedOperations = append([]Operation(nil), c.SupportedOperations...)
	}
	if c.InputModalities != nil {
		clone.InputModalities = append([]Modality(nil), c.InputModalities...)
	}
	if c.OutputModalities != nil {
		clone.OutputModalities = append([]Modality(nil), c.OutputModalities...)
	}
	if c.GenerationParameters != nil {
		clone.GenerationParameters = append([]GenerationParameter(nil), c.GenerationParameters...)
	}
	return clone
}

// SupportsOperation checks if the capability set supports the requested operation.
func (c CapabilitySet) SupportsOperation(op Operation) bool {
	for _, candidate := range c.SupportedOperations {
		if candidate == op {
			return true
		}
	}
	return false
}

// SupportsToolProtocol fails closed for every normalized tool protocol requirement.
func (c CapabilitySet) SupportsToolProtocol(requirements llm.ToolProtocolRequirements, stream bool) bool {
	return !requirements.UsesProtocol() || c.ToolCalling && c.ToolProtocol.Supports(requirements, stream)
}

// SatisfiesRequirements checks legacy stream, tool, and image requirements.
func (c CapabilitySet) SatisfiesRequirements(requiresStream, requiresTools, requiresImage bool) bool {
	if requiresStream && !c.Streaming || requiresTools && !c.ToolCalling {
		return false
	}
	if !requiresImage {
		return true
	}
	for _, modality := range c.InputModalities {
		if modality == ModalityImage {
			return true
		}
	}
	return false
}
