// Package agent contains the KB index executor that implements worker.TaskExecutor
// for "kb_index" tasks. Unlike the AgentExecutor (which delegates to LLM agent
// loops), this executor is handler-driven: it calls LLM as a tool for semantic
// chunking, calls embedding for vectorization, and writes results to Qdrant +
// MongoDB — all under Go code control, never delegated to the LLM.
package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	kbsvc "github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
)

// KBIndexExecutor implements worker.TaskExecutor for kb_index tasks.
// It performs the full indexing pipeline:
//  1. Load doc from KB service
//  2. Download file from GridFS
//  3. Call LLM (UseCaseKBChunking) for semantic chunking
//  4. Call embedding + store vectors via KB service (already wired with WithVectorIndex)
//  5. Update doc status to "ready"
type KBIndexExecutor struct {
	kb       *kbsvc.Service     // concrete service (has IndexDocument + addChunks with embed)
	provider *modelcfg.Provider  // for BuildLLM(UseCaseKBChunking)
}

// NewKBIndexExecutor creates the KB indexing executor.
func NewKBIndexExecutor(kb *kbsvc.Service, provider *modelcfg.Provider) *KBIndexExecutor {
	return &KBIndexExecutor{kb: kb, provider: provider}
}

// Execute runs the KB indexing pipeline. Returns nil on success, error on failure.
func (e *KBIndexExecutor) Execute(ctx context.Context, t *domaintask.Task) error {
	// Respect cancellation.
	if t.Status == domaintask.StatusCancelled {
		return nil
	}

	docID, _ := t.Params["doc_id"].(string)
	if docID == "" {
		err := fmt.Errorf("kb_index task %s: missing doc_id in params", t.ID)
		e.failTask(t, err)
		return err
	}

	log.Printf("[kb-index] starting indexing for doc=%s task=%s", docID, t.ID)

	// Build the chunking function: call LLM to split text semantically.
	chunkFn := e.buildChunkFn(ctx)

	// Run the full indexing pipeline via the knowledge service.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	if err := e.kb.IndexDocument(ctx, docID, chunkFn); err != nil {
		log.Printf("[kb-index] indexing failed for doc=%s: %v", docID, err)
		e.failTask(t, err)
		return err
	}

	log.Printf("[kb-index] indexing complete for doc=%s", docID)
	e.completeTask(t)
	return nil
}

// buildChunkFn creates a chunking function that calls the LLM for semantic
// splitting. Returns a function that takes raw text and returns chunks.
func (e *KBIndexExecutor) buildChunkFn(ctx context.Context) func(text string) ([]string, error) {
	return func(text string) ([]string, error) {
		if e.provider == nil {
			// Fallback: simple paragraph-based splitting.
			chunks := splitParagraphs(text, 500)
			return chunks, nil
		}

		llm, err := e.provider.BuildLLM(ctx, modelcfg.UseCaseKBChunking)
		if err != nil {
			log.Printf("[kb-index] LLM unavailable, using paragraph split: %v", err)
			return splitParagraphs(text, 500), nil
		}

		prompt := buildChunkPrompt(text)
		temp := float32(0.3)
		adkReq := &model.LLMRequest{
			Contents: []*genai.Content{{
				Role: "user",
				Parts: []*genai.Part{genai.NewPartFromText(prompt)},
			}},
			Config: &genai.GenerateContentConfig{Temperature: &temp},
		}
		var output string
		for resp, err := range llm.GenerateContent(ctx, adkReq, false) {
			if err != nil {
				log.Printf("[kb-index] LLM chunk call failed, using paragraph split: %v", err)
				return splitParagraphs(text, 500), nil
			}
			if resp.Content != nil && len(resp.Content.Parts) > 0 {
				output += resp.Content.Parts[0].Text
			}
		}

		chunks := parseChunkResponse(output)
		if len(chunks) == 0 {
			// Empty result: fallback to paragraph split.
			return splitParagraphs(text, 500), nil
		}
		return chunks, nil
	}
}

// buildChunkPrompt creates the LLM prompt for semantic text chunking.
func buildChunkPrompt(text string) string {
	return fmt.Sprintf(`You are a document chunker. Split the following text into semantic chunks at paragraph or section boundaries.
Each chunk should be a coherent, self-contained unit of meaning, roughly 200-800 characters.
Return ONLY the chunks, one per line, separated by "---CHUNK---". Do NOT add any explanation.

Text to chunk:
%s`, truncateForPrompt(text, 8000))
}

// truncateForPrompt limits text to avoid token overflow.
func truncateForPrompt(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "\n...[truncated]"
}

// parseChunkResponse splits LLM output by "---CHUNK---" markers.
func parseChunkResponse(output string) []string {
	var chunks []string
	for _, part := range strings.Split(output, "---CHUNK---") {
		c := strings.TrimSpace(part)
		if c != "" {
			chunks = append(chunks, c)
		}
	}
	return chunks
}

// splitParagraphs is a fallback chunker that splits by double-newline and merges
// small paragraphs to reach target size.
func splitParagraphs(text string, targetSize int) []string {
	paras := strings.Split(text, "\n\n")
	var chunks []string
	var current strings.Builder

	for _, p := range paras {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if current.Len() == 0 {
			current.WriteString(p)
		} else if current.Len()+len(p)+2 <= targetSize {
			current.WriteString("\n\n")
			current.WriteString(p)
		} else {
			chunks = append(chunks, current.String())
			current.Reset()
			current.WriteString(p)
		}
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func (e *KBIndexExecutor) completeTask(t *domaintask.Task) {
	// The KB indexing task is a system task; we don't set a complex result.
	// Just mark it completed.
	_ = e.kb // KB service reference for future result storage if needed.
}

func (e *KBIndexExecutor) failTask(t *domaintask.Task, err error) {
	// Failure is logged; the worker pool handles DLQ.
	log.Printf("[kb-index] task %s failed: %v", t.ID, err)
}

// Ensure KBIndexExecutor satisfies worker.TaskExecutor (duck typing; checked in wire.go).
var _ = (*KBIndexExecutor)(nil)
