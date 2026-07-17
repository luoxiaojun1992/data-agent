package agent

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
)

func TestNewManager(t *testing.T) {
	t.Run("short key padded", func(t *testing.T) {
		m, err := NewManager("short")
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		if m == nil {
			t.Fatal("should return non-nil manager")
		}
	})

	t.Run("exact 32 byte key", func(t *testing.T) {
		m, err := NewManager("12345678901234567890123456789012")
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		if m == nil {
			t.Fatal("should return non-nil manager")
		}
	})
}

func TestEncryptDecrypt(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	plaintext := "hello, world!"
	encrypted, err := m.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if encrypted == "" {
		t.Fatal("encrypted should not be empty")
	}

	decrypted, err := m.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("roundtrip failed: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncrypt_AesNewCipherError(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	patches := gomonkey.ApplyFunc(aes.NewCipher, func(key []byte) (cipher.Block, error) {
		return nil, fmt.Errorf("mock aes error")
	})
	defer patches.Reset()

	_, err = m.Encrypt("test")
	if err == nil {
		t.Fatal("should error on aes.NewCipher failure")
	}
}

func TestEncrypt_NewGCMError(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	patches := gomonkey.ApplyFunc(cipher.NewGCM, func(block cipher.Block) (cipher.AEAD, error) {
		return nil, fmt.Errorf("mock gcm error")
	})
	defer patches.Reset()

	_, err = m.Encrypt("test")
	if err == nil {
		t.Fatal("should error on cipher.NewGCM failure")
	}
}

func TestEncrypt_ReadFullError(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	patches := gomonkey.ApplyFunc(io.ReadFull, func(r io.Reader, buf []byte) (int, error) {
		return 0, fmt.Errorf("mock readfull error")
	})
	defer patches.Reset()

	_, err = m.Encrypt("test")
	if err == nil {
		t.Fatal("should error on io.ReadFull failure")
	}
}


func TestDecrypt_GCMOpenError(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// Encrypt a value first
	encrypted, _ := m.Encrypt("testdata")
	// Mock cipher.NewGCM to return a mock AEAD that fails on Open
	patches := gomonkey.ApplyFunc(cipher.NewGCM, func(block cipher.Block) (cipher.AEAD, error) {
		return &mockAEAD{failOpen: true}, nil
	})
	defer patches.Reset()
	_, err = m.Decrypt(encrypted)
	if err == nil {
		t.Fatal("should error on GCM Open failure")
	}
}

type mockAEAD struct{ failOpen bool }

func (m *mockAEAD) NonceSize() int                                { return 12 }
func (m *mockAEAD) Overhead() int                                 { return 16 }
func (m *mockAEAD) Seal(dst, nonce, plaintext, additionalData []byte) []byte { return nil }
func (m *mockAEAD) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if m.failOpen {
		return nil, fmt.Errorf("mock open error")
	}
	return nil, nil
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	_, err = m.Decrypt("not-valid-base64!!!")
	if err == nil {
		t.Fatal("should error on invalid base64")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	_, err = m.Decrypt("YWJj")
	if err == nil {
		t.Fatal("should error on short ciphertext")
	}
}

func TestDecrypt_AesNewCipherError(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	patches := gomonkey.ApplyFunc(aes.NewCipher, func(key []byte) (cipher.Block, error) {
		return nil, fmt.Errorf("mock aes error")
	})
	defer patches.Reset()

	// Need a valid base64 string to pass the decode step
	_, err = m.Decrypt("dGVzdA==") // "test" in base64
	if err == nil {
		t.Fatal("should error on aes.NewCipher failure in Decrypt")
	}
}

func TestDecrypt_NewGCMError(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	patches := gomonkey.ApplyFunc(cipher.NewGCM, func(block cipher.Block) (cipher.AEAD, error) {
		return nil, fmt.Errorf("mock gcm error")
	})
	defer patches.Reset()

	_, err = m.Decrypt("dGVzdA==")
	if err == nil {
		t.Fatal("should error on cipher.NewGCM failure in Decrypt")
	}
}

func TestStoreRetrieve(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	err = m.Store("mykey", "myvalue")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	val, err := m.Retrieve("mykey")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if val != "myvalue" {
		t.Errorf("got %q, want %q", val, "myvalue")
	}

	_, err = m.Retrieve("nonexistent")
	if err == nil {
		t.Fatal("should error for nonexistent key")
	}
}

func TestStore_EncryptError(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	patches := gomonkey.ApplyFunc(aes.NewCipher, func(key []byte) (cipher.Block, error) {
		return nil, fmt.Errorf("mock aes error")
	})
	defer patches.Reset()

	err = m.Store("key", "value")
	if err == nil {
		t.Fatal("should error on encrypt failure in Store")
	}
}


func TestRotateKey_WithSecrets(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	err = m.Store("key1", "value1")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	err = m.Store("key2", "value2")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	err = m.RotateKey("abcdefghijklmnopqrstuvwxyz123456")
	if err != nil {
		t.Fatalf("RotateKey with secrets: %v", err)
	}
	// Verify secrets still retrievable after rotation
	val, err := m.Retrieve("key1")
	if err != nil {
		t.Fatalf("Retrieve after rotation: %v", err)
	}
	if val != "value1" {
		t.Errorf("got %q, want %q", val, "value1")
	}
}

func TestRotateKey(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	oldKey := make([]byte, len(m.masterKey))
	copy(oldKey, m.masterKey)

	err = m.RotateKey("abcdefghijklmnopqrstuvwxyz123456") // 32 bytes
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}

	if string(m.masterKey) == string(oldKey) {
		t.Error("key should change after rotation")
	}
}

func TestRotateKey_ShortKey(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	err = m.RotateKey("short")
	if err != nil {
		t.Fatalf("RotateKey with short key: %v", err)
	}
}

func TestRotateKey_EncryptError(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Store a secret with old key
	err = m.Store("key", "val")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Mock aes.NewCipher to fail → decrypt fails first
	patches := gomonkey.ApplyFunc(aes.NewCipher, func(key []byte) (cipher.Block, error) {
		return nil, fmt.Errorf("mock aes error during rotation")
	})
	defer patches.Reset()

	err = m.RotateKey("abcdefghijklmnopqrstuvwxyz123456")
	if err == nil {
		t.Fatal("should error when decryption fails during rotation")
	}
}

func TestRotateKey_ReEncryptError(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	err = m.Store("key1", "plaintext-value")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	// mock io.ReadFull to fail → encrypt (new key) fails on nonce gen,
	// but decrypt (old key) doesn't call io.ReadFull so it succeeds
	patches := gomonkey.ApplyFunc(io.ReadFull, func(r io.Reader, buf []byte) (int, error) {
		return 0, fmt.Errorf("mock nonce error")
	})
	defer patches.Reset()
	err = m.RotateKey("abcdefghijklmnopqrstuvwxyz123456")
	if err == nil {
		t.Fatal("should error when re-encryption fails during rotation")
	}
}