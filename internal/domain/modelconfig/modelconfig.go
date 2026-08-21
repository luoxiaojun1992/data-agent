// Package modelconfig defines the model-configuration domain types (entities,
// value objects, and database model structures) shared by the repository,
// provider, and handler layers. It is deliberately a leaf domain package: it
// must not import infra packages (mongo-driver, adk, etc.).
package modelconfig

// ModelType distinguishes LLM and Embedding models.
type ModelType string

const (
	ModelTypeLLM       ModelType = "llm"
	ModelTypeEmbedding ModelType = "embedding"
)

// UseCase identifies the intended use for a model.
type UseCase string

const (
	UseCaseChat           UseCase = "chat"
	UseCaseTask           UseCase = "task"
	UseCaseEnhance        UseCase = "enhance"
	UseCaseCompaction     UseCase = "compaction"
	UseCaseKBChunking     UseCase = "kb_chunking"
	UseCaseKBImage        UseCase = "kb_image"
	UseCaseEmbedding      UseCase = "embedding"
	UseCaseIntentCheck    UseCase = "intent_check"
	UseCaseRelevanceCheck UseCase = "relevance_check"
)

// validUseCases is the authoritative set of legal use-case values. Adding a
// new use case requires appending it here AND adding the corresponding UseCase
// constant above.
var validUseCases = []UseCase{
	UseCaseChat,
	UseCaseTask,
	UseCaseEnhance,
	UseCaseCompaction,
	UseCaseKBChunking,
	UseCaseKBImage,
	UseCaseEmbedding,
	UseCaseIntentCheck,
	UseCaseRelevanceCheck,
}

// ValidUseCases returns the full set of legal use-case values (LLM + embedding).
// The API input layer uses this to reject unknown use-case strings.
func ValidUseCases() []UseCase {
	out := make([]UseCase, len(validUseCases))
	copy(out, validUseCases)
	return out
}

// IsValidUseCase reports whether s is a legal use-case value.
func IsValidUseCase(s string) bool {
	for _, uc := range validUseCases {
		if string(uc) == s {
			return true
		}
	}
	return false
}

// ModelEntry is the database model of one configured model (LLM or embedding).
// IsDefault is gone; IsDefaultFor is a response-only derived field (no bson tag
// — it is assembled from model_defaults at read time, never persisted here).
type ModelEntry struct {
	ID              string    `json:"id" bson:"_id"` // pure UUID, no "model_" prefix
	Name            string    `json:"name" bson:"name"`
	BaseURL         string    `json:"base_url" bson:"base_url"`
	APIKey          string    `json:"api_key,omitempty" bson:"api_key,omitempty"` // Vault path (plaintext only in memory)
	Type            ModelType `json:"type" bson:"type"`
	Instruction     string    `json:"instruction" bson:"instruction,omitempty"`
	Capability      string    `json:"capability" bson:"capability,omitempty"`
	UseCases        []string  `json:"use_cases" bson:"use_cases,omitempty"`
	TokenMultiplier float64   `json:"token_multiplier" bson:"token_multiplier"`
	Temperature     float64   `json:"temperature" bson:"temperature"`
	MaxTokens       int       `json:"max_tokens" bson:"max_tokens"`
	ContextLen      int       `json:"context_len" bson:"context_len"`
	IsDefaultFor    []string  `json:"is_default_for,omitempty"` // response-only, assembled from model_defaults
	FallbackOrder   int       `json:"fallback_order" bson:"fallback_order"`
	EmbeddingDim    int       `json:"embedding_dim" bson:"embedding_dim,omitempty"`
}

// ModelDefault maps one use case to its default model.
type ModelDefault struct {
	ID      string `json:"id" bson:"_id"`            // pure UUID (non-semantic)
	UseCase string `json:"use_case" bson:"use_case"` // business field, unique index
	ModelID string `json:"model_id" bson:"model_id"` // ModelEntry.ID
}

// MongoDB collection names for model config storage.
const (
	CollModelConfigs  = "model_configs"
	CollModelDefaults = "model_defaults"
)
