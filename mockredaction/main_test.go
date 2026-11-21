package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetect_PhoneNumber(t *testing.T) {
	got := detect("手机 13812345678")
	if len(got) != 1 {
		t.Fatalf("results = %d, want 1", len(got))
	}
	r := got[0]
	if r.EntityType != "PHONE_NUMBER" || r.Start != 3 || r.End != 14 {
		t.Errorf("got entity=%s [%d,%d), want PHONE_NUMBER [3,14)", r.EntityType, r.Start, r.End)
	}
}

func TestDetect_LeadingDigitBoundary(t *testing.T) {
	// A leading digit must NOT be treated as part of the phone number; the
	// (?<!\d) lookaround from custom_recognizers.yaml means no match here.
	got := detect("013812345678")
	for _, r := range got {
		if r.EntityType == "PHONE_NUMBER" {
			t.Errorf("unexpected PHONE_NUMBER match in leading-digit text: %+v", r)
		}
	}
}

func TestDetect_IDCardNotAlsoBankCard(t *testing.T) {
	// An 18-digit ID card must be matched once as CN_ID_CARD, not also as a
	// 16-19 digit CN_BANK_CARD (overlap dedup).
	got := detect("320123199001011234")
	if len(got) != 1 {
		t.Fatalf("results = %d, want 1 (dedup)", len(got))
	}
	if got[0].EntityType != "CN_ID_CARD" {
		t.Errorf("entity = %s, want CN_ID_CARD", got[0].EntityType)
	}
	if got[0].Start != 0 || got[0].End != 18 {
		t.Errorf("span = [%d,%d), want [0,18)", got[0].Start, got[0].End)
	}
}

func TestDetect_Email(t *testing.T) {
	got := detect("联系 a@b.com")
	if len(got) != 1 || got[0].EntityType != "EMAIL_ADDRESS" {
		t.Fatalf("got %+v, want single EMAIL_ADDRESS", got)
	}
}

func TestRedact_Single(t *testing.T) {
	got := redact("手机 13812345678", []analyzeResult{{EntityType: "PHONE_NUMBER", Start: 3, End: 14}}, "<PII>")
	if got != "手机 <PII>" {
		t.Errorf("got %q, want %q", got, "手机 <PII>")
	}
}

func TestRedact_Multiple(t *testing.T) {
	text := "手机13812345678，身份证320123199001011234"
	results := detect(text)
	got := redact(text, results, "<PII>")
	want := "手机<PII>，身份证<PII>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(results) != 2 {
		t.Errorf("results = %d, want 2", len(results))
	}
}

func TestRedact_NoResults(t *testing.T) {
	if got := redact("no pii here", nil, "<PII>"); got != "no pii here" {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestDetect_SuffixDigitBoundary(t *testing.T) {
	// A trailing digit must NOT be treated as part of the phone number (the
	// (?!\d) lookaround).
	got := detect("138123456780")
	for _, r := range got {
		if r.EntityType == "PHONE_NUMBER" {
			t.Errorf("unexpected PHONE_NUMBER match with trailing digit: %+v", r)
		}
	}
}

func TestDetect_BankCard(t *testing.T) {
	got := detect("卡号 6222021234567890")
	if len(got) != 1 || got[0].EntityType != "CN_BANK_CARD" {
		t.Fatalf("got %+v, want single CN_BANK_CARD", got)
	}
}

func TestRedact_InvalidSpan(t *testing.T) {
	// Invalid spans (start<0, start>=end) are skipped, leaving text unchanged.
	got := redact("abc", []analyzeResult{{Start: -1, End: 1}}, "<PII>")
	if got != "abc" {
		t.Errorf("got %q, want unchanged (invalid span skipped)", got)
	}
	got = redact("abc", []analyzeResult{{Start: 2, End: 2}}, "<PII>")
	if got != "abc" {
		t.Errorf("got %q, want unchanged (empty span skipped)", got)
	}
}

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	healthHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestAnalyzeHandler(t *testing.T) {
	body := `{"text":"手机 13812345678","language":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/analyze", strings.NewReader(body))
	rr := httptest.NewRecorder()
	analyzeHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "PHONE_NUMBER") {
		t.Errorf("body = %q, want PHONE_NUMBER result", rr.Body.String())
	}
}

func TestAnonymizeHandler(t *testing.T) {
	body := `{"text":"手机 13812345678","analyzer_results":[{"entity_type":"PHONE_NUMBER","start":3,"end":14,"score":0.9}],"anonymizers":{"DEFAULT":{"type":"replace","new_value":"<PII>"}}}`
	req := httptest.NewRequest(http.MethodPost, "/anonymize", strings.NewReader(body))
	rr := httptest.NewRecorder()
	anonymizeHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "手机 <PII>") {
		t.Errorf("body = %q, want redacted text", rr.Body.String())
	}
}

func TestAnalyzeHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/analyze", nil)
	rr := httptest.NewRecorder()
	analyzeHandler(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestAnalyzeHandler_BadRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/analyze", strings.NewReader("{invalid"))
	rr := httptest.NewRecorder()
	analyzeHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestAnonymizeHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/anonymize", nil)
	rr := httptest.NewRecorder()
	anonymizeHandler(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestAnonymizeHandler_BadRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/anonymize", strings.NewReader("{invalid"))
	rr := httptest.NewRecorder()
	anonymizeHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}
