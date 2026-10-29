package apicollection

import (
	"encoding/json"
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
)

func TestNewService(t *testing.T) {
	repo := &mongo.APICollectionRepo{}
	s := NewService(repo)
	if s == nil || s.repo != repo {
		t.Error("NewService() did not retain the provided repo")
	}
}

func TestParseOpenAPISpec_Empty(t *testing.T) {
	if got := parseOpenAPISpec(nil); got != nil {
		t.Errorf("parseOpenAPISpec(nil) = %v, want nil", got)
	}
	if got := parseOpenAPISpec(json.RawMessage{}); got != nil {
		t.Errorf("parseOpenAPISpec(empty) = %v, want nil", got)
	}
}

func TestParseOpenAPISpec_InvalidJSON(t *testing.T) {
	if got := parseOpenAPISpec(json.RawMessage(`{invalid`)); got != nil {
		t.Errorf("parseOpenAPISpec(invalid) = %v, want nil", got)
	}
}

func TestParseOpenAPISpec_Valid(t *testing.T) {
	m := parseOpenAPISpec(json.RawMessage(`{"openapi":"3.0.0","paths":{"/a":{}}}`))
	if m == nil {
		t.Fatal("parseOpenAPISpec(valid) returned nil")
	}
	if m["openapi"] != "3.0.0" {
		t.Errorf("openapi = %v, want 3.0.0", m["openapi"])
	}
}
