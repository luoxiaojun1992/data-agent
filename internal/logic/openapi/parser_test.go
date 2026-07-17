package openapi

import (
	"testing"
)

func TestParse_ReturnsNonNil(t *testing.T) {
	def, err := Parse([]byte(`{"openapi": "3.0"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def == nil {
		t.Fatal("expected non-nil APIDef")
	}
	if def.Title != "API Definition" {
		t.Errorf("Title: got %q, want %q", def.Title, "API Definition")
	}
	if def.Version != "1.0" {
		t.Errorf("Version: got %q, want %q", def.Version, "1.0")
	}
}

func TestParse_EmptyInput(t *testing.T) {
	def, err := Parse([]byte{})
	if err != nil {
		t.Fatalf("unexpected error on empty input: %v", err)
	}
	if def == nil {
		t.Fatal("expected non-nil even for empty input")
	}
}

func TestEndpoint_ToMCPTool(t *testing.T) {
	e := &Endpoint{
		Path:    "/users",
		Method:  "GET",
		Summary: "List users",
		Parameters: []Param{
			{Name: "page", In: "query", Required: false, Type: "integer"},
			{Name: "limit", In: "query", Required: true, Type: "integer"},
		},
	}
	tool := e.ToMCPTool()
	if tool == nil {
		t.Fatal("expected non-nil tool")
	}
	if tool["name"] != "List users" {
		t.Errorf("name: got %v, want 'List users'", tool["name"])
	}
	if tool["description"] != "List users" {
		t.Errorf("description: got %v, want 'List users'", tool["description"])
	}
	schema, ok := tool["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("expected inputSchema to be a map")
	}
	if schema["type"] != "object" {
		t.Errorf("inputSchema.type: got %v, want object", schema["type"])
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected inputSchema.properties to be a map")
	}
	if len(props) != 2 {
		t.Fatalf("expected 2 properties, got %d", len(props))
	}
	pageProp := props["page"].(map[string]interface{})
	if pageProp["type"] != "integer" {
		t.Errorf("page.type: got %v, want integer", pageProp["type"])
	}
}

func TestEndpoint_ToMCPTool_NoParams(t *testing.T) {
	e := &Endpoint{
		Path:    "/health",
		Method:  "GET",
		Summary: "Health check",
	}
	tool := e.ToMCPTool()
	schema, ok := tool["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("expected inputSchema")
	}
	props := schema["properties"].(map[string]interface{})
	if len(props) != 0 {
		t.Errorf("expected 0 properties, got %d", len(props))
	}
}

func TestParamsToSchema(t *testing.T) {
	params := []Param{
		{Name: "id", In: "path", Required: true, Type: "string"},
		{Name: "name", In: "query", Required: false, Type: "string"},
	}
	schema := paramsToSchema(params)
	if len(schema) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(schema))
	}
	idSchema := schema["id"].(map[string]interface{})
	if idSchema["type"] != "string" {
		t.Errorf("id.type: got %v, want string", idSchema["type"])
	}
}

func TestParamsToSchema_Empty(t *testing.T) {
	schema := paramsToSchema(nil)
	if len(schema) != 0 {
		t.Errorf("expected empty schema for nil params, got %d", len(schema))
	}
}
