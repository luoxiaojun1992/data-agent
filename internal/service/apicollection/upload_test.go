package apicollection

import (
	"context"
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
)

// CreateUpload validates the OpenAPI payload *before* touching the repository,
// so the two validation-failure branches are testable with a zero-value repo
// (never invoked). The success path needs a live MongoDB and is out of scope.

func TestCreateUpload_InvalidJSON(t *testing.T) {
	s := NewService(&mongo.APICollectionRepo{})
	_, err := s.CreateUpload(context.Background(), "u1", "n", "d", []byte(`{invalid`), "f1")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestCreateUpload_MissingOpenapiVersion(t *testing.T) {
	s := NewService(&mongo.APICollectionRepo{})
	_, err := s.CreateUpload(context.Background(), "u1", "n", "d", []byte(`{"paths":{}}`), "f1")
	if err == nil {
		t.Fatal("expected error for missing openapi version")
	}
}
