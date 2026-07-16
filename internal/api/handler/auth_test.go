package handler

import (
	"testing"
)

func TestNewAuthHandler(t *testing.T) {
	h := NewAuthHandler(nil)
	if h == nil {
		t.Fatal("NewAuthHandler() should not return nil")
	}
}

func TestParseInt64(t *testing.T) {
	t.Run("valid positive", func(t *testing.T) {
		v, err := parseInt64("42")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 42 {
			t.Errorf("parseInt64('42') = %d, want 42", v)
		}
	})

	t.Run("valid negative", func(t *testing.T) {
		v, err := parseInt64("-10")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != -10 {
			t.Errorf("parseInt64('-10') = %d, want -10", v)
		}
	})

	t.Run("zero", func(t *testing.T) {
		v, err := parseInt64("0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 0 {
			t.Errorf("parseInt64('0') = %d, want 0", v)
		}
	})

	t.Run("large number", func(t *testing.T) {
		v, err := parseInt64("9999999999")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 9999999999 {
			t.Errorf("parseInt64('9999999999') = %d, want 9999999999", v)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		_, err := parseInt64("")
		if err == nil {
			t.Error("parseInt64('') should return error")
		}
	})

	t.Run("non-numeric", func(t *testing.T) {
		_, err := parseInt64("abc")
		if err == nil {
			t.Error("parseInt64('abc') should return error")
		}
	})

	t.Run("decimal", func(t *testing.T) {
		_, err := parseInt64("3.14")
		if err == nil {
			t.Error("parseInt64('3.14') should return error")
		}
	})

	t.Run("negative", func(t *testing.T) {
		v, err := parseInt64("-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != -1 {
			t.Errorf("parseInt64('-1') = %d, want -1", v)
		}
	})
}
