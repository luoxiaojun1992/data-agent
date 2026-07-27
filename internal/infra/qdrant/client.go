package qdrant

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client provides access to Qdrant vector database via REST API.
type Client struct {
	addr   string
	client *http.Client
}

// NewClient creates a Qdrant HTTP client.
func NewClient(addr string) *Client {
	return &Client{
		addr:   strings.TrimRight(addr, "/"),
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Point is a vector point in Qdrant.
type Point struct {
	ID      int64          `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload,omitempty"`
}

// UpsertPoints inserts or updates points in a collection.
func (c *Client) UpsertPoints(collection string, points []Point) error {
	payload := map[string]any{"points": points}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal points: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points?wait=true", c.addr, collection)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("upsert points: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant upsert %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// SearchHit represents a vector search result.
type SearchHit struct {
	ID      int64          `json:"id"`
	Score   float32        `json:"score"`
	Version int            `json:"version,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

// CreateCollection creates a Qdrant collection for storing vectors.
func (c *Client) CreateCollection(collection string, vectorSize int, distance string) error {
	if distance == "" {
		distance = "Cosine"
	}
	payload := map[string]any{
		"vectors": map[string]any{
			"size":     vectorSize,
			"distance": distance,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal collection config: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s", c.addr, collection)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create collection request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant create collection %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// HasCollection checks whether a Qdrant collection exists.
func (c *Client) HasCollection(collection string) (bool, error) {
	url := fmt.Sprintf("%s/collections/%s", c.addr, collection)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// Search performs a vector similarity search (REST POST).
func (c *Client) Search(collection string, vector []float32, topK int) ([]SearchHit, error) {
	payload := map[string]any{
		"vector":       vector,
		"limit":        topK,
		"with_payload": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal search: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points/search", c.addr, collection)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search qdrant: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("qdrant search %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result []SearchHit `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}
	return result.Result, nil
}
