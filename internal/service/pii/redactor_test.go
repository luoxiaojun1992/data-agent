package pii

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// failingTransport always fails the HTTP round trip, exercising the
// network-error branches of analyze/anonymize/health.
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection refused")
}

func TestNew_DefaultsClientWhenNil(t *testing.T) {
	r := New(Config{AnalyzerURL: "http://a", AnonymizerURL: "http://b"})
	if r == nil {
		t.Fatal("New() returned nil")
	}
	if r.client == nil {
		t.Fatal("New() should default a nil HTTPClient to a real client")
	}
}

func TestNew_UsesProvidedClient(t *testing.T) {
	client := &http.Client{}
	r := New(Config{HTTPClient: client})
	if r.client != client {
		t.Error("New() did not use the provided HTTPClient")
	}
}

func TestRedact_EmptyText(t *testing.T) {
	r := New(Config{})
	got, err := r.Redact(context.Background(), "")
	if err != nil {
		t.Fatalf("empty text should not error, got %v", err)
	}
	if got != "" {
		t.Errorf("empty text returned %q", got)
	}
}

func TestRedact_Disabled(t *testing.T) {
	r := New(Config{Enabled: func() bool { return false }})
	_, err := r.Redact(context.Background(), "hello")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestRedact_Unconfigured(t *testing.T) {
	r := New(Config{AnalyzerURL: "http://a"}) // anonymizer URL missing
	_, err := r.Redact(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error for unconfigured services")
	}
}

func TestRedact_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/analyze":
			w.Write([]byte(`[{"entity_type":"PHONE_NUMBER","start":0,"end":13,"score":0.9}]`))
		case "/anonymize":
			w.Write([]byte(`{"text":"<PII>","items":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	r := New(Config{AnalyzerURL: srv.URL, AnonymizerURL: srv.URL, HTTPClient: srv.Client()})
	got, err := r.Redact(context.Background(), "13812345678")
	if err != nil {
		t.Fatalf("Redact() error = %v", err)
	}
	if got != "<PII>" {
		t.Errorf("Redact() = %q, want %q", got, "<PII>")
	}
}

func TestRedact_NoPII(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/analyze" {
			w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := New(Config{AnalyzerURL: srv.URL, AnonymizerURL: srv.URL, HTTPClient: srv.Client()})
	got, err := r.Redact(context.Background(), "no sensitive data here")
	if err != nil {
		t.Fatalf("Redact() error = %v", err)
	}
	if got != "no sensitive data here" {
		t.Errorf("Redact() = %q, want original text", got)
	}
}

func TestRedact_AnalyzeFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := New(Config{AnalyzerURL: srv.URL, AnonymizerURL: srv.URL, HTTPClient: srv.Client()})
	_, err := r.Redact(context.Background(), "13812345678")
	if err == nil {
		t.Fatal("expected error when analyze fails")
	}
}

func TestAnalyze_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	r := New(Config{AnalyzerURL: srv.URL, HTTPClient: srv.Client()})
	_, err := r.analyze(context.Background(), "text")
	if err == nil {
		t.Fatal("expected error for non-200 analyze status")
	}
}

func TestAnalyze_NetworkError(t *testing.T) {
	r := New(Config{AnalyzerURL: "http://127.0.0.1:1", HTTPClient: &http.Client{Transport: failingTransport{}}})
	_, err := r.analyze(context.Background(), "text")
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestAnalyze_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	r := New(Config{AnalyzerURL: srv.URL, HTTPClient: srv.Client()})
	_, err := r.analyze(context.Background(), "text")
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestAnonymize_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	r := New(Config{AnonymizerURL: srv.URL, HTTPClient: srv.Client()})
	_, err := r.anonymize(context.Background(), "text", []analyzeResult{{EntityType: "PHONE_NUMBER", Start: 0, End: 13}})
	if err == nil {
		t.Fatal("expected error for non-200 anonymize status")
	}
}

func TestAnonymize_NetworkError(t *testing.T) {
	r := New(Config{AnonymizerURL: "http://127.0.0.1:1", HTTPClient: &http.Client{Transport: failingTransport{}}})
	_, err := r.anonymize(context.Background(), "text", []analyzeResult{{}})
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestAnonymize_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	r := New(Config{AnonymizerURL: srv.URL, HTTPClient: srv.Client()})
	_, err := r.anonymize(context.Background(), "text", []analyzeResult{{}})
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestHealth_Unconfigured(t *testing.T) {
	r := New(Config{})
	if err := r.Health(context.Background()); err == nil {
		t.Fatal("expected error for unconfigured services")
	}
}

func TestHealth_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := New(Config{AnalyzerURL: srv.URL, AnonymizerURL: srv.URL, HTTPClient: srv.Client()})
	if err := r.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
}

func TestHealth_AnalyzerFailure(t *testing.T) {
	// analyzer returns non-200, anonymizer is healthy.
	analyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer analyzer.Close()
	anonymizer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer anonymizer.Close()

	r := New(Config{AnalyzerURL: analyzer.URL, AnonymizerURL: anonymizer.URL, HTTPClient: http.DefaultClient})
	if err := r.Health(context.Background()); err == nil {
		t.Fatal("expected error when analyzer is unhealthy")
	}
}

func TestHealth_AnonymizerFailure(t *testing.T) {
	analyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer analyzer.Close()
	anonymizer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer anonymizer.Close()

	r := New(Config{AnalyzerURL: analyzer.URL, AnonymizerURL: anonymizer.URL, HTTPClient: http.DefaultClient})
	if err := r.Health(context.Background()); err == nil {
		t.Fatal("expected error when anonymizer is unhealthy")
	}
}

func TestHealthEndpoint_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if err := healthEndpoint(context.Background(), http.DefaultClient, srv.URL); err == nil {
		t.Fatal("expected error for non-200 health status")
	}
}

func TestHealthEndpoint_NetworkError(t *testing.T) {
	err := healthEndpoint(context.Background(), &http.Client{Transport: failingTransport{}}, "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected network error")
	}
}
