package openapi

import (
	"testing"
)

func TestParse(t *testing.T) {
	t.Run("empty json returns defaults", func(t *testing.T) {
		def, err := Parse([]byte(`{}`))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		if def.Title != "API Definition" {
			t.Errorf("Title: got %q", def.Title)
		}
		if def.Version != "1.0" {
			t.Errorf("Version: got %q", def.Version)
		}
	})

	t.Run("nil input still works", func(t *testing.T) {
		def, err := Parse(nil)
		if err != nil {
			t.Fatalf("Parse nil: %v", err)
		}
		if def == nil {
			t.Fatal("should return non-nil def")
		}
	})
}

func TestEndpoint_ToMCPTool(t *testing.T) {
	e := &Endpoint{
		Path:    "/users",
		Method:  "GET",
		Summary: "List Users",
		Parameters: []Param{
			{Name: "limit", In: "query", Required: false, Type: "integer"},
		},
	}

	tool := e.ToMCPTool()
	if tool["name"] != "List Users" {
		t.Errorf("name: got %v", tool["name"])
	}
	if tool["description"] != "List Users" {
		t.Errorf("description: got %v", tool["description"])
	}

	inputSchema, ok := tool["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("inputSchema should be map")
	}
	if inputSchema["type"] != "object" {
		t.Errorf("schema type: got %v", inputSchema["type"])
	}
}

func TestParamsToSchema(t *testing.T) {
	t.Run("empty params", func(t *testing.T) {
		schema := paramsToSchema(nil)
		if len(schema) != 0 {
			t.Errorf("empty params: got %d entries", len(schema))
		}
	})

	t.Run("with params", func(t *testing.T) {
		params := []Param{
			{Name: "id", In: "path", Required: true, Type: "string"},
			{Name: "page", In: "query", Required: false, Type: "integer"},
		}
		schema := paramsToSchema(params)
		if len(schema) != 2 {
			t.Errorf("with params: got %d entries, want 2", len(schema))
		}
	})
}
