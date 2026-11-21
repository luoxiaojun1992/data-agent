// mockredaction is a lightweight PII redaction service for UI tests (SPEC-107).
// It replaces the real presidio analyzer/anonymizer with a deterministic,
// regex-only implementation, exposing the same HTTP contract that the backend
// PIIRedactor (internal/service/pii/redactor.go) depends on:
//
//	POST /analyze    {"text","language"} -> [{"entity_type","start","end","score"}]
//	POST /anonymize  {"text","analyzer_results","anonymizers"} -> {"text","items"}
//	GET  /health     -> 200
//
// Unlike presidio (spaCy NER + gunicorn sync worker, which occasionally hangs
// on CI), this service has no model to load and starts instantly, so UI tests
// no longer flake on "worker never boots".
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// rule is a single PII detection rule. re is a Go regexp (RE2, no lookaround),
// and the prefix/suffix exclusion sets implement the lookaround boundaries from
// docker/presidio/custom_recognizers.yaml (Python's \b treats CJK as word chars,
// so the original rules use (?<!\d)...(?!\d) instead).
type rule struct {
	entityType    string
	re            *regexp.Regexp
	prefixExclude string // chars that must NOT precede the match ("" = no check)
	suffixExclude string // chars that must NOT follow the match ("" = no check)
}

// rules are ordered by specificity: a more specific recognizer runs first and
// a later, broader one skips any span already claimed (e.g. a Chinese ID card
// should not also be matched as a bank card).
var rules = []rule{
	{"EMAIL_ADDRESS", regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`), "", ""},
	{"CN_ID_CARD", regexp.MustCompile(`\d{17}[\dXx]`), "0123456789", "0123456789"},
	{"PHONE_NUMBER", regexp.MustCompile(`1[3-9]\d{9}`), "0123456789", "0123456789"},
	{"CN_USCC", regexp.MustCompile(`[0-9A-HJ-NPQRTUWXY]{18}`), "0123456789ABCDEFGHJKLMNPQRTUWXY", "0123456789ABCDEFGHJKLMNPQRTUWXY"},
	{"CN_BANK_CARD", regexp.MustCompile(`\d{16,19}`), "0123456789", "0123456789"},
}

// analyzeResult mirrors the presidio analyzer response item consumed by
// PIIRedactor.analyze (internal/service/pii/redactor.go).
type analyzeResult struct {
	EntityType string  `json:"entity_type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Score      float64 `json:"score"`
}

// analyzeRequest is the body of POST /analyze.
type analyzeRequest struct {
	Text     string `json:"text"`
	Language string `json:"language"`
}

// anonymizeRequest is the body of POST /anonymize.
type anonymizeRequest struct {
	Text            string          `json:"text"`
	AnalyzerResults []analyzeResult `json:"analyzer_results"`
	Anonymizers     map[string]struct {
		Type     string `json:"type"`
		NewValue string `json:"new_value"`
	} `json:"anonymizers"`
}

// anonymizeResponse is the body of the POST /anonymize response.
type anonymizeResponse struct {
	Text  string `json:"text"`
	Items []any  `json:"items"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/analyze", analyzeHandler)
	mux.HandleFunc("/anonymize", anonymizeHandler)
	mux.HandleFunc("/health", healthHandler)

	port := envOrDefault("PORT", "8083")
	log.Printf("mockredaction listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func analyzeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req analyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, detect(req.Text))
}

func anonymizeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req anonymizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	newValue := "<PII>"
	if a, ok := req.Anonymizers["DEFAULT"]; ok && a.NewValue != "" {
		newValue = a.NewValue
	}
	writeJSON(w, http.StatusOK, anonymizeResponse{Text: redact(req.Text, req.AnalyzerResults, newValue), Items: []any{}})
}

// detect finds all PII spans in text, returning rune-based [start,end) indices
// with a fixed score. Boundaries mirror custom_recognizers.yaml's lookarounds.
func detect(text string) []analyzeResult {
	runes := []rune(text)
	var results []analyzeResult
	for _, r := range rules {
		for _, m := range r.re.FindAllStringIndex(text, -1) {
			start := utf8.RuneCountInString(text[:m[0]])
			end := utf8.RuneCountInString(text[:m[1]])
			if start > 0 && r.prefixExclude != "" && strings.ContainsRune(r.prefixExclude, runes[start-1]) {
				continue
			}
			if end < len(runes) && r.suffixExclude != "" && strings.ContainsRune(r.suffixExclude, runes[end]) {
				continue
			}
			if overlaps(results, start, end) {
				continue
			}
			results = append(results, analyzeResult{EntityType: r.entityType, Start: start, End: end, Score: 0.9})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Start < results[j].Start })
	return results
}

// overlaps reports whether [start,end) intersects any existing result.
func overlaps(results []analyzeResult, start, end int) bool {
	for _, e := range results {
		if start < e.End && end > e.Start {
			return true
		}
	}
	return false
}

// redact replaces the spans in results (rune-based) with newValue, processing
// spans from right to left so earlier indices stay valid.
func redact(text string, results []analyzeResult, newValue string) string {
	if len(results) == 0 {
		return text
	}
	runes := []rune(text)
	spans := append([]analyzeResult(nil), results...)
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start > spans[j].Start })
	for _, s := range spans {
		if s.Start < 0 || s.End > len(runes) || s.Start >= s.End {
			continue
		}
		runes = append(runes[:s.Start], append([]rune(newValue), runes[s.End:]...)...)
	}
	return string(runes)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // keep "<PII>" literal, matching presidio's output
	if err := enc.Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
