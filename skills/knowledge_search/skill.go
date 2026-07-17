package skill

import (
	"fmt"
	"strings"

	skilldomain "github.com/luoxiaojun1992/data-agent/internal/domain/skill"
	knowledgepkg "github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
)

// KnowledgeSearch implements skill.Skill for knowledge base search.
type KnowledgeSearch struct {
	kbService *knowledgepkg.Service
}

// NewKnowledgeSearch creates a knowledge search skill backed by the KB service.
func NewKnowledgeSearch(kbService *knowledgepkg.Service) *KnowledgeSearch {
	return &KnowledgeSearch{kbService: kbService}
}

func (s *KnowledgeSearch) Name() string { return "knowledge_search" }
func (s *KnowledgeSearch) Description() string {
	return "Searches the knowledge base with full-text and semantic search capabilities"
}

func (s *KnowledgeSearch) Parameters() []skilldomain.Parameter {
	return []skilldomain.Parameter{
		{Name: "query", Type: "string", Description: "Search query string", Required: true},
		{Name: "top_k", Type: "integer", Description: "Maximum number of results (default: 5)", Required: false},
		{Name: "role", Type: "string", Description: "User role for permission filtering", Required: false},
	}
}

func (s *KnowledgeSearch) Permissions() []string {
	return []string{"skill:knowledge_search"}
}

func (s *KnowledgeSearch) RateLimit() *skilldomain.RateLimitConfig {
	return &skilldomain.RateLimitConfig{MaxRequests: 30, WindowSec: 60}
}

func (s *KnowledgeSearch) Execute(ctx skilldomain.SkillContext, params map[string]any) (any, error) {
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("knowledge_search: missing required parameter 'query'")
	}

	topK := 5
	if raw, ok := params["top_k"]; ok {
		switch n := raw.(type) {
		case float64:
			topK = int(n)
		case int:
			topK = n
		}
	}
	if topK < 1 {
		topK = 1
	}
	if topK > 50 {
		topK = 50
	}

	role := ctx.Role
	if raw, ok := params["role"].(string); ok && raw != "" {
		role = raw
	}

	results, err := s.kbService.Search(ctx.UserID, query, topK, role)
	if err != nil {
		return nil, fmt.Errorf("knowledge_search: search failed: %w", err)
	}

	// Format results for LLM consumption
	var formatted []map[string]any
	for _, r := range results {
		formatted = append(formatted, map[string]any{
			"doc_id":  r.DocID,
			"title":   r.DocTitle,
			"content": truncateContent(r.Content, 500),
			"score":   r.Score,
		})
	}

	return map[string]any{
		"query":   query,
		"results": formatted,
		"count":   len(formatted),
	}, nil
}

func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return strings.TrimSpace(content[:maxLen]) + "..."
}
