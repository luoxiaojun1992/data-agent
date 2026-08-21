package guard

import (
	"context"
	"encoding/json"
	"strings"

	"google.golang.org/adk/model"
)

// generateText runs a one-shot LLM request and returns the trimmed text of the
// first response with non-empty content. Returns an error on any failure.
func generateText(ctx context.Context, llm model.LLM, req *model.LLMRequest) (string, error) {
	for resp, err := range llm.GenerateContent(ctx, req, false) {
		if err != nil {
			return "", err
		}
		if resp == nil || resp.Content == nil {
			continue
		}
		var sb strings.Builder
		for _, p := range resp.Content.Parts {
			if p != nil {
				sb.WriteString(p.Text)
			}
		}
		if s := strings.TrimSpace(sb.String()); s != "" {
			return s, nil
		}
	}
	return "", nil
}

// stripCodeFence removes a ```json ... ``` (or ``` ... ```) wrapper and
// surrounding whitespace so a bare JSON object can be unmarshalled.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}

// parseIsTask parses the intent-check JSON response, defaulting to chat
// (false) on any parse failure so a malformed response never blocks the flow.
func parseIsTask(s string) bool {
	s = stripCodeFence(s)
	var v struct {
		IsTask bool `json:"is_task"`
	}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return false
	}
	return v.IsTask
}

// parseIsRelevant parses the relevance-check JSON response, defaulting to
// relevant (true) on any parse failure so a malformed response never blocks
// the flow (and never triggers a spurious retry).
func parseIsRelevant(s string) bool {
	s = stripCodeFence(s)
	var v struct {
		IsRelevant bool `json:"is_relevant"`
	}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return true
	}
	return v.IsRelevant
}
