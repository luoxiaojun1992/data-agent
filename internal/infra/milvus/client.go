package milvus

import (
	"fmt"
)

// Client provides access to Milvus vector database (stub for MVP).
type Client struct {
	addr string
}

// NewClient creates a Milvus client.
func NewClient(addr string) *Client {
	return &Client{addr: addr}
}

// Search performs a vector similarity search (stub).
func (c *Client) Search(collection string, vector []float64, topK int) ([]SearchHit, error) {
	_ = collection
	_ = vector
	_ = topK
	// Placeholder — full Milvus integration in SPEC-009
	return nil, fmt.Errorf("milvus not yet integrated (SPEC-009)")
}

// SearchHit represents a vector search result.
type SearchHit struct {
	ID     int64   `json:"id"`
	Score  float32 `json:"score"`
	Fields map[string]interface{} `json:"fields"`
}
