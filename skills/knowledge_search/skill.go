package knowledge_search

import (
	"fmt"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

type Searcher struct {
	service interface {
		Search(userID, query string, topK int, role string) ([]interface{}, error)
	}
}

func New(service interface{ Search(string, string, int, string) ([]interface{}, error) }) *Searcher {
	return &Searcher{service: service}
}

func (s *Searcher) Name() string        { return "knowledge_search" }
func (s *Searcher) Description() string { return "Searches the knowledge base using hybrid search (semantic + full-text)" }

func (s *Searcher) Parameters() []skill.Parameter {
	return []skill.Parameter{
		{Name: "query", Type: "string", Description: "Search query", Required: true},
		{Name: "top_k", Type: "integer", Description: "Number of results to return (default 5)", Required: false},
	}
}

func (s *Searcher) Permissions() []string               { return []string{"kb:manage_own"} }
func (s *Searcher) RateLimit() *skill.RateLimitConfig   { return &skill.RateLimitConfig{MaxRequests: 20, WindowSec: 60} }

func (s *Searcher) Execute(ctx skill.SkillContext, params map[string]any) (any, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query required")
	}

	topK := 5
	if k, ok := params["top_k"].(float64); ok {
		topK = int(k)
	}

	results, err := s.service.Search(ctx.UserID, query, topK, ctx.Role)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return map[string]interface{}{
		"query":   query,
		"results": results,
	}, nil
}
