package modelconfig

import "testing"

func TestValidUseCases_ReturnsAllUseCases(t *testing.T) {
	got := ValidUseCases()

	want := []UseCase{
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
	if len(got) != len(want) {
		t.Fatalf("ValidUseCases() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ValidUseCases()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestValidUseCases_ReturnsCopy(t *testing.T) {
	first := ValidUseCases()
	if len(first) == 0 {
		t.Fatal("ValidUseCases() returned empty")
	}
	// Mutating the returned slice must not affect subsequent calls.
	first[0] = UseCase("mutated")
	second := ValidUseCases()
	if second[0] == UseCase("mutated") {
		t.Error("ValidUseCases() leaked its internal slice")
	}
}

func TestIsValidUseCase(t *testing.T) {
	for _, uc := range ValidUseCases() {
		if !IsValidUseCase(string(uc)) {
			t.Errorf("IsValidUseCase(%q) = false, want true", uc)
		}
	}

	for _, invalid := range []string{"", "unknown", "LLM", " CHAT", "chat ", "kb"} {
		if IsValidUseCase(invalid) {
			t.Errorf("IsValidUseCase(%q) = true, want false", invalid)
		}
	}
}

func TestConstants(t *testing.T) {
	if ModelTypeLLM != "llm" || ModelTypeEmbedding != "embedding" {
		t.Errorf("ModelType constants unexpected: llm=%q embedding=%q", ModelTypeLLM, ModelTypeEmbedding)
	}
	if CollModelConfigs != "model_configs" || CollModelDefaults != "model_defaults" {
		t.Errorf("collection constants unexpected: %q %q", CollModelConfigs, CollModelDefaults)
	}
}
