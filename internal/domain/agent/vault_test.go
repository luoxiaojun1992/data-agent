package agent

import (
	"testing"
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
	m, err := NewManager("12345678901234567890123456789012") // exactly 32 bytes
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

func TestRotateKey(t *testing.T) {
	m, err := NewManager("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	oldKey := make([]byte, len(m.masterKey))
	copy(oldKey, m.masterKey)

	err = m.RotateKey("new-secret-key-for-rotation-test!")
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

	// Short key gets padded to 32 bytes
	err = m.RotateKey("short")
	if err != nil {
		t.Fatalf("RotateKey with short key: %v", err)
	}
}
