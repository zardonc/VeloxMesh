package providers

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

// CapabilitySet describes the supported capabilities of a provider adapter.
type CapabilitySet struct {
	ProviderType         ProviderType
	SupportedOperations  []Operation
	InputModalities      []Modality
	OutputModalities     []Modality
	Streaming            bool
	ToolCalling          bool
	GenerationParameters []GenerationParameter
	// Add optional constraints here if needed in the future
}

// Clone returns a deep copy of the CapabilitySet.
func (c CapabilitySet) Clone() CapabilitySet {
	clone := CapabilitySet{
		ProviderType: c.ProviderType,
		Streaming:    c.Streaming,
		ToolCalling:  c.ToolCalling,
	}

	if c.SupportedOperations != nil {
		clone.SupportedOperations = make([]Operation, len(c.SupportedOperations))
		copy(clone.SupportedOperations, c.SupportedOperations)
	}

	if c.InputModalities != nil {
		clone.InputModalities = make([]Modality, len(c.InputModalities))
		copy(clone.InputModalities, c.InputModalities)
	}

	if c.OutputModalities != nil {
		clone.OutputModalities = make([]Modality, len(c.OutputModalities))
		copy(clone.OutputModalities, c.OutputModalities)
	}

	if c.GenerationParameters != nil {
		clone.GenerationParameters = make([]GenerationParameter, len(c.GenerationParameters))
		copy(clone.GenerationParameters, c.GenerationParameters)
	}

	return clone
}

// SupportsOperation checks if the capability set supports the requested operation.
func (c CapabilitySet) SupportsOperation(op Operation) bool {
	for _, o := range c.SupportedOperations {
		if o == op {
			return true
		}
	}
	return false
}

// SatisfiesRequirements checks if the capability set satisfies the given requirements.
// Currently checks Streaming, ToolCalling, and InputModalities based on the provided booleans.
func (c CapabilitySet) SatisfiesRequirements(requiresStream, requiresTools, requiresImage bool) bool {
	if requiresStream && !c.Streaming {
		return false
	}
	if requiresTools && !c.ToolCalling {
		return false
	}
	if requiresImage {
		found := false
		for _, m := range c.InputModalities {
			if m == ModalityImage {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
