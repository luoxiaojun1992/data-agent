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
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	knowledge "github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	kbsvc "github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
)

// KBIndexExecutor implements worker.TaskExecutor for kb_index runs.
type KBIndexExecutor struct {
	kb       *kbsvc.Service     // concrete service
	provider *modelcfg.Provider // for BuildLLM(UseCaseKBChunking)
	runSvc   domaintask.TaskRunService
}

// NewKBIndexExecutor creates the KB indexing executor.
func NewKBIndexExecutor(kb *kbsvc.Service, provider *modelcfg.Provider, runSvc domaintask.TaskRunService) *KBIndexExecutor {
	return &KBIndexExecutor{kb: kb, provider: provider, runSvc: runSvc}
}

// Execute runs the KB indexing pipeline. Returns nil on success, error on failure.
func (e *KBIndexExecutor) Execute(ctx context.Context, run *domaintask.TaskRun) error {
	// Respect cancellation.
	if run.Status == domaintask.StatusCancelled {
		return nil
	}

	docID, _ := run.Params["doc_id"].(string)
	if docID == "" {
		err := fmt.Errorf("kb_index run %s: missing doc_id in params", run.ID)
		e.failRun(run, err)
		return err
	}

	log.Printf("[kb-index] starting indexing for doc=%s run=%s", docID, run.ID)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// Branch on file_type: images are parsed by a multimodal LLM first, then
	// indexed through the same TXT pipeline; documents go straight to text
	// indexing.
	doc, err := e.kb.GetDoc(docID)
	if err != nil {
		log.Printf("[kb-index] get doc failed for doc=%s: %v", docID, err)
		e.failRun(run, err)
		return err
	}

	if knowledge.IsImage(doc.FileType) {
		err = e.indexImage(ctx, docID, doc)
	} else {
		// Build the chunking function: call LLM to split text semantically.
		chunkFn := e.buildChunkFn(ctx)
		err = e.kb.IndexDocument(ctx, docID, chunkFn)
	}
	if err != nil {
		log.Printf("[kb-index] indexing failed for doc=%s: %v", docID, err)
		e.failRun(run, err)
		return err
	}

	log.Printf("[kb-index] indexing complete for doc=%s", docID)
	e.completeRun(run)
	return nil
}

// indexImage parses an image document via the multimodal LLM and then indexes
// the resulting text through the standard TXT pipeline.
func (e *KBIndexExecutor) indexImage(ctx context.Context, docID string, doc *knowledge.KnowledgeDoc) error {
	// 1. Download the image bytes from GridFS.
	data, err := e.kb.DownloadFile(ctx, doc.GridFSFileID)
	if err != nil {
		return fmt.Errorf("download image %s: %w", doc.GridFSFileID, err)
	}

	// 2. Multimodal LLM parse: image → natural-language description.
	text, err := e.parseImage(ctx, data, doc.FileName)
	if err != nil {
		return fmt.Errorf("parse image %s: %w", docID, err)
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("parse image %s: empty result", docID)
	}

	// 3. Index the parsed text via the shared TXT pipeline.
	chunkFn := e.buildChunkFn(ctx)
	return e.kb.IndexContent(ctx, docID, text, chunkFn)
}

// parseImage sends image bytes (as base64 inline data) plus a description
// prompt to the KB chunking LLM and returns the textual description.
func (e *KBIndexExecutor) parseImage(ctx context.Context, data []byte, fileName string) (string, error) {
	if e.provider == nil {
		return "", fmt.Errorf("no LLM provider configured for image parsing")
	}
	llm, err := e.provider.BuildLLM(ctx, modelcfg.UseCaseKBImage)
	if err != nil {
		return "", err
	}

	mimeType := inferImageMimeType(fileName)
	adkReq := &model.LLMRequest{
		Contents: []*genai.Content{{
			Role: "user",
			Parts: []*genai.Part{
				genai.NewPartFromBytes(data, mimeType),
				genai.NewPartFromText(imageParsePrompt),
			},
		}},
	}

	var output string
	for resp, err := range llm.GenerateContent(ctx, adkReq, false) {
		if err != nil {
			return "", err
		}
		if resp.Content != nil && len(resp.Content.Parts) > 0 {
			output += resp.Content.Parts[0].Text
		}
	}
	return strings.TrimSpace(output), nil
}

// imageParsePrompt instructs the model to describe the image as indexable text.
const imageParsePrompt = "请详细描述这张图片的内容，包括其中的文字、数据、图表、对象及其含义，输出可直接用于知识库检索的中文描述。"

// inferImageMimeType maps a file extension to an image MIME type.
func inferImageMimeType(fileName string) string {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return "image/png"
	}
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

func (e *KBIndexExecutor) completeRun(run *domaintask.TaskRun) {
	_ = e.runSvc.UpdateRunResult(run.ID, map[string]interface{}{"content": "kb_indexing_complete", "status": "success"})
}

func (e *KBIndexExecutor) failRun(run *domaintask.TaskRun, err error) {
	_ = e.runSvc.UpdateRunError(run.ID, err.Error())
	log.Printf("[kb-index] run %s failed: %v", run.ID, err)
}

// Ensure KBIndexExecutor satisfies worker.TaskExecutor (duck typing; checked in wire.go).
var _ = (*KBIndexExecutor)(nil)
