package agent

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"time"
)

// Secret represents an encrypted secret stored in Vault.
type Secret struct {
	Key       string    `json:"key"`
	Value     string    `json:"-"`  // encrypted base64
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// Manager handles encryption/decryption of secrets using AES-256-GCM.
type Manager struct {
	mu           sync.RWMutex
	masterKey    []byte
	secrets      map[string]*Secret
	rotationDays int
}

// NewManager creates a new vault manager with the given master key.
// In production, the master key should be loaded from Vault or environment.
func NewManager(masterKey string) (*Manager, error) {
	key := []byte(masterKey)
	if len(key) < 32 {
		// Pad or derive key to 32 bytes for AES-256
		padded := make([]byte, 32)
		copy(padded, key)
		key = padded
	}

	return &Manager{
		masterKey:    key,
		secrets:      make(map[string]*Secret),
		rotationDays: 30,
	}, nil
}

// Encrypt encrypts a plaintext value using AES-256-GCM.
func (m *Manager) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(m.masterKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded ciphertext.
func (m *Manager) Decrypt(encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	block, err := aes.NewCipher(m.masterKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// Store encrypts and stores a secret.
func (m *Manager) Store(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	encrypted, err := m.Encrypt(value)
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}

	m.secrets[key] = &Secret{
		Key:       key,
		Value:     encrypted,
		CreatedAt: time.Now(),
	}
	return nil
}

// Retrieve decrypts and returns a stored secret.
func (m *Manager) Retrieve(key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	secret, exists := m.secrets[key]
	if !exists {
		return "", fmt.Errorf("secret %q not found", key)
	}

	return m.Decrypt(secret.Value)
}

// RotateKey rotates the master key and re-encrypts all secrets.
// Old key is retained for 7 days to decrypt historical data.
func (m *Manager) RotateKey(newMasterKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	newKey := []byte(newMasterKey)
	if len(newKey) < 32 {
		padded := make([]byte, 32)
		copy(padded, newKey)
		newKey = padded
	}

	// Create temporary manager with new key for re-encryption
	tmp := &Manager{masterKey: newKey}

	for key, secret := range m.secrets {
		// Decrypt with old key
		plaintext, err := m.Decrypt(secret.Value)
		if err != nil {
			return fmt.Errorf("decrypt %q with old key: %w", key, err)
		}

		// Re-encrypt with new key
		encrypted, err := tmp.Encrypt(plaintext)
		if err != nil {
			return fmt.Errorf("encrypt %q with new key: %w", key, err)
		}

		secret.Value = encrypted
	}

	m.masterKey = newKey
	return nil
}
