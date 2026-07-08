package chat

import (
	"strings"
	"unicode/utf8"

	"github.com/luoxiaojun1992/data-agent/internal/domain/agent"
)

// ContextManager handles context window management and compression.
type ContextManager struct {
	maxTokens int
	threshold float64 // 0.0 to 1.0, default 0.5
}

// NewContextManager creates a context manager with the given max tokens and compression threshold.
func NewContextManager(maxTokens int, threshold float64) *ContextManager {
	return &ContextManager{
		maxTokens: maxTokens,
		threshold: threshold,
	}
}

// ShouldCompress checks if the current token count exceeds the threshold.
func (cm *ContextManager) ShouldCompress(currentTokens int) bool {
	if cm.maxTokens <= 0 {
		return false
	}
	return float64(currentTokens) > float64(cm.maxTokens)*cm.threshold
}

// EstimateTokens provides a rough token count estimate (~4 chars per token for English, ~2 for CJK).
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	charCount := utf8.RuneCountInString(text)
	// Rough heuristic: CJK characters ~2 chars/token, ASCII ~4 chars/token
	cjkCount := 0
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			cjkCount++
		}
	}
	ascCount := charCount - cjkCount
	return (cjkCount/2 + ascCount/4) + 1
}

// TruncateMessages truncates a list of messages to fit within maxTokens.
// Keeps system message + last N messages that fit.
func (cm *ContextManager) TruncateMessages(messages []agent.Message, maxTokens int) []agent.Message {
	if len(messages) == 0 || maxTokens <= 0 {
		return messages
	}

	// Always keep the first system message
	var sysMsg *agent.Message
	var rest []agent.Message
	for i, m := range messages {
		if m.Role == "system" && sysMsg == nil {
			sysMsg = &messages[i]
		} else {
			rest = append(rest, m)
		}
	}

	totalTokens := 0
	if sysMsg != nil {
		totalTokens += EstimateTokens(sysMsg.Content)
	}

	// Add messages from the end until we hit the limit
	var kept []agent.Message
	for i := len(rest) - 1; i >= 0; i-- {
		tok := EstimateTokens(rest[i].Content)
		if totalTokens+tok > maxTokens {
			break
		}
		totalTokens += tok
		kept = append([]agent.Message{rest[i]}, kept...)
	}

	result := kept
	if sysMsg != nil {
		result = append([]agent.Message{*sysMsg}, kept...)
	}
	return result
}

// CompressSummary generates a summary prompt for context compression.
func CompressSummary(messages []agent.Message) string {
	var sb strings.Builder
	sb.WriteString("Previous conversation summary:\n")
	for _, m := range messages {
		if m.Role != "system" {
			sb.WriteString(m.Role)
			sb.WriteString(": ")
			content := m.Content
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			sb.WriteString(content)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
