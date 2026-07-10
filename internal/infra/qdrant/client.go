package qdrant

import (
	"fmt"
)

// Client provides access to Qdrant vector database (stub for MVP).
type Client struct {
	addr string
}

// NewClient creates a Qdrant client (gRPC, port 6334).
func NewClient(addr string) *Client {
	return &Client{addr: addr}
}

// Search performs a vector similarity search (stub).
func (c *Client) Search(collection string, vector []float64, topK int) ([]SearchHit, error) {
	_ = collection
	_ = vector
	_ = topK
	return nil, fmt.Errorf("qdrant not yet integrated")
}

// SearchHit represents a vector search result.
type SearchHit struct {
	ID     int64                  `json:"id"`
	Score  float32                `json:"score"`
	Fields map[string]interface{} `json:"fields"`
}
