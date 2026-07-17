package openapi

// APIDef represents a parsed OpenAPI definition.
type APIDef struct {
	Title     string     `json:"title"`
	Version   string     `json:"version"`
	BaseURL   string     `json:"base_url"`
	Endpoints []Endpoint `json:"endpoints"`
}

// Endpoint represents an API endpoint.
type Endpoint struct {
	Path       string  `json:"path"`
	Method     string  `json:"method"`
	Summary    string  `json:"summary"`
	Parameters []Param `json:"parameters"`
}

// Param represents an API parameter.
type Param struct {
	Name     string `json:"name"`
	In       string `json:"in"` // query, path, header
	Required bool   `json:"required"`
	Type     string `json:"type"`
}

// Parse accepts raw OpenAPI JSON and returns a parsed definition.
func Parse(rawJSON []byte) (*APIDef, error) {
	// Simplified parser — full implementation uses kin-openapi or similar
	_ = rawJSON
	return &APIDef{
		Title:   "API Definition",
		Version: "1.0",
	}, nil
}

// ToMCPTool converts an endpoint to an MCP tool definition.
func (e *Endpoint) ToMCPTool() map[string]interface{} {
	return map[string]interface{}{
		"name":        e.Summary,
		"description": e.Summary,
		"inputSchema": map[string]interface{}{
			"type":       "object",
			"properties": paramsToSchema(e.Parameters),
		},
	}
}

func paramsToSchema(params []Param) map[string]interface{} {
	schema := make(map[string]interface{})
	for _, p := range params {
		schema[p.Name] = map[string]interface{}{
			"type":        p.Type,
			"description": p.Name,
		}
	}
	return schema
}
