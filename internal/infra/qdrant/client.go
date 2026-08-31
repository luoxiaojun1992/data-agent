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

// Search performs a vector similarity search (REST POST). The optional
// filter is forwarded to Qdrant's /points/search so permission scoping
// (e.g. creator_id / is_public) is enforced server-side.
func (c *Client) Search(collection string, vector []float32, topK int, filter map[string]any) ([]SearchHit, error) {
	payload := map[string]any{
		"vector":       vector,
		"limit":        topK,
		"with_payload": true,
	}
	if len(filter) > 0 {
		payload["filter"] = filter
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

// SetPayload updates payload fields on points matching the given filter.
func (c *Client) SetPayload(collection string, payload map[string]any, filter map[string]any) error {
	bodyPayload := map[string]any{
		"payload": payload,
		"filter":  filter,
	}
	body, err := json.Marshal(bodyPayload)
	if err != nil {
		return fmt.Errorf("marshal set payload: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points/payload?wait=true", c.addr, collection)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create set payload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("set payload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant set payload %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// DeletePoints deletes all points matching the given filter. Deleting zero
// points is not an error — the operation is idempotent (SPEC-070 cascade
// delete).
func (c *Client) DeletePoints(collection string, filter map[string]any) error {
	payload := map[string]any{"filter": filter}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal delete points: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points/delete?wait=true", c.addr, collection)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create delete points request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("delete points: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant delete points %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// RetrievePoints fetches points by numeric IDs together with their payloads
// (content lookup for the graph search skill — SPEC-070). Missing points are
// simply absent from the result, so callers degrade gracefully.
func (c *Client) RetrievePoints(collection string, ids []int64) (map[int64]map[string]any, error) {
	if len(ids) == 0 {
		return map[int64]map[string]any{}, nil
	}
	payload := map[string]any{
		"ids":          ids,
		"with_payload": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal retrieve points: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points", c.addr, collection)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create retrieve points request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("retrieve points: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("qdrant retrieve points %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result []struct {
			ID      int64          `json:"id"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode retrieve points: %w", err)
	}
	out := make(map[int64]map[string]any, len(result.Result))
	for _, r := range result.Result {
		out[r.ID] = r.Payload
	}
	return out, nil
}
