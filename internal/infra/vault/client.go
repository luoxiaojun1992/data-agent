package vault

import (
	"context"
	"fmt"
	"os"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

// Client wraps the HashiCorp Vault API client for secret management.
type Client struct {
	client *vault.Client
	mount  string // KV v2 mount path, e.g. "secret"
}

// NewClient creates a new Vault client using VAULT_ADDR and VAULT_TOKEN env vars.
func NewClient() (*Client, error) {
	config := vault.DefaultConfig()
	if addr := os.Getenv("VAULT_ADDR"); addr != "" {
		config.Address = addr
	}

	vclient, err := vault.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("create vault client: %w", err)
	}

	// Use VAULT_TOKEN env var; if not set, fall back to VAULT_DEV_ROOT_TOKEN_ID for dev
	token := os.Getenv("VAULT_TOKEN")
	if token == "" {
		token = os.Getenv("VAULT_DEV_ROOT_TOKEN_ID")
	}
	if token != "" {
		vclient.SetToken(token)
	}

	return &Client{
		client: vclient,
		mount:  "secret",
	}, nil
}

// Store writes a secret value to Vault KV v2 at the given path.
// path should be like "data-agent/api_key" (without mount prefix).
func (c *Client) Store(ctx context.Context, path, value string) error {
	if c == nil {
		return fmt.Errorf("vault client receiver is nil")
	}
	if c.client == nil {
		return fmt.Errorf("vault client inner client is nil")
	}
	data := map[string]interface{}{
		"data": map[string]interface{}{
			path: value,
		},
	}

	// KV v2 path: {mount}/data/{path} (Vault Go client adds /v1 prefix)
	fullPath := fmt.Sprintf("%s/data/%s", c.mount, path)
	_, err := c.client.Logical().WriteWithContext(ctx, fullPath, data)
	if err != nil {
		return fmt.Errorf("vault store %s: %w", path, err)
	}
	return nil
}

// Retrieve reads a secret value from Vault KV v2 at the given path.
func (c *Client) Retrieve(ctx context.Context, path string) (string, error) {
	if c == nil || c.client == nil {
		return "", fmt.Errorf("vault client not initialized")
	}
	fullPath := fmt.Sprintf("%s/data/%s", c.mount, path)
	secret, err := c.client.Logical().ReadWithContext(ctx, fullPath)
	if err != nil {
		return "", fmt.Errorf("vault retrieve %s: %w", path, err)
	}
	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("vault: secret %q not found", path)
	}

	// KV v2 wraps data in a "data" map
	dataMap, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("vault: unexpected secret format for %q", path)
	}

	value, ok := dataMap[path].(string)
	if !ok {
		// Try any key
		for _, v := range dataMap {
			if s, ok := v.(string); ok {
				return s, nil
			}
		}
		return "", fmt.Errorf("vault: secret %q has no string value", path)
	}
	return value, nil
}

// HasSecret checks if a secret exists at the given path.
func (c *Client) HasSecret(ctx context.Context, path string) bool {
	val, err := c.Retrieve(ctx, path)
	return err == nil && val != ""
}

// NormalizeAPIKey converts a raw API key to a deterministic Vault path.
func APIKeyPath(namespace string) string {
	// Store model API keys under secret/data-agent/model_api_key
	return fmt.Sprintf("%s/model_api_key", namespace)
}

// HermesAPIKeyPath returns the Vault path for Hermes API key.
func HermesAPIKeyPath(namespace string) string {
	return fmt.Sprintf("%s/hermes_api_key", namespace)
}

// GetAddr returns the Vault server address from environment.
func GetAddr() string {
	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		addr = "http://localhost:8200"
	}
	return addr
}

// Ping verifies Vault reachability via sys/health (SPEC-079 health probe,
// aligned with the docker-compose healthcheck `vault status`). Returns an error
// when the server is unreachable or reports uninitialized.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("vault client not initialized")
	}
	if c.IsAvailable(ctx) {
		return nil
	}
	return fmt.Errorf("vault unavailable")
}

// IsAvailable checks if Vault is reachable, initialized and unsealed by calling sys/health.
// Uses Sys().HealthWithContext because sys/health returns a flat JSON body (not the
// {data:...} secret envelope), which Logical().Read does not decode into resp.Data.
func (c *Client) IsAvailable(ctx context.Context) bool {
	if c == nil || c.client == nil {
		return false
	}
	health, err := c.client.Sys().HealthWithContext(ctx)
	if err != nil || health == nil {
		return false
	}
	return health.Initialized && !health.Sealed
}

// MaskValue returns a masked representation of a sensitive value.
func MaskValue(val string) string {
	if len(val) <= 4 {
		return strings.Repeat("•", len(val))
	}
	return val[:2] + strings.Repeat("•", max(len(val)-4, 1)) + val[len(val)-2:]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
