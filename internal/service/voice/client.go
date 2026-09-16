package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrUnavailable means the whisper sidecar is unreachable or not configured.
var ErrUnavailable = errors.New("voice service unavailable")

// Client fronts the whisper sidecar's POST /transcribe endpoint.
type Client struct {
	endpoint string
	token    string
	client   *http.Client
}

// ClientConfig configures the sidecar HTTP client.
type ClientConfig struct {
	Endpoint   string        // sidecar /transcribe URL
	Token      string        // optional X-Internal-Token header value
	Timeout    time.Duration // transcription timeout
	HTTPClient *http.Client  // overrides the default client (tests)
}

// NewClient creates a sidecar client. An empty Endpoint yields an inert client
// whose Transcribe always returns ErrUnavailable.
func NewClient(cfg ClientConfig) *Client {
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	return &Client{endpoint: cfg.Endpoint, token: cfg.Token, client: client}
}

type transcribeRequest struct {
	Audio      string `json:"audio"`
	Lang       string `json:"lang"`
	SampleRate int    `json:"sample_rate"`
}

type transcribeResponse struct {
	Text  string `json:"text"`
	Error string `json:"error"`
}

// Transcribe sends the merged int16 PCM (16 kHz mono) to the sidecar and
// returns the raw transcription text.
func (c *Client) Transcribe(ctx context.Context, audio []byte, lang string) (string, error) {
	if c == nil || c.endpoint == "" {
		return "", ErrUnavailable
	}
	body, err := json.Marshal(transcribeRequest{
		Audio:      base64.StdEncoding.EncodeToString(audio),
		Lang:       lang,
		SampleRate: 16000,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("X-Internal-Token", c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	var out transcribeResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", errors.New(out.Error)
	}
	return out.Text, nil
}
