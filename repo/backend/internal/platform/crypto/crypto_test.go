package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}
	os.Setenv("ENCRYPTION_KEY", hex.EncodeToString(key))
	return key
}

func TestEncryptDecrypt(t *testing.T) {
	key := testKey(t)
	plaintext := "+1-555-867-5309"

	ct, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	got, err := Decrypt(key, ct)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if got != plaintext {
		t.Fatalf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key := testKey(t)
	plaintext := "sensitive-data"

	ct, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	wrongKey := make([]byte, 32)
	if _, err := rand.Read(wrongKey); err != nil {
		t.Fatalf("failed to generate wrong key: %v", err)
	}

	_, err = Decrypt(wrongKey, ct)
	if err == nil {
		t.Fatal("expected Decrypt to fail with wrong key, but it succeeded")
	}
}

func TestEncryptProducesDifferentCiphertext(t *testing.T) {
	key := testKey(t)
	plaintext := "same-input"

	ct1, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("first Encrypt failed: %v", err)
	}

	ct2, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("second Encrypt failed: %v", err)
	}

	if ct1 == ct2 {
		t.Fatal("two encryptions of the same plaintext produced identical ciphertext; nonce reuse detected")
	}

	// Both should still decrypt to the same plaintext
	p1, _ := Decrypt(key, ct1)
	p2, _ := Decrypt(key, ct2)
	if p1 != plaintext || p2 != plaintext {
		t.Fatal("decrypted values do not match original plaintext")
	}
}

func TestKeyFromEnv(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	hexKey := hex.EncodeToString(key)
	os.Setenv("ENCRYPTION_KEY", hexKey)

	got, err := Key()
	if err != nil {
		t.Fatalf("Key() failed: %v", err)
	}
	if hex.EncodeToString(got) != hexKey {
		t.Fatal("Key() returned wrong value")
	}
}

func TestKeyMissing(t *testing.T) {
	os.Unsetenv("ENCRYPTION_KEY")
	_, err := Key()
	if err == nil {
		t.Fatal("expected error when ENCRYPTION_KEY is not set")
	}
}

func TestMaskPhone(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"+1-555-867-5309", "***5309"},
		{"5551234", "***1234"},
		{"123", "***123"},
		{"", ""},
	}
	for _, tc := range tests {
		got := MaskPhone(tc.input)
		if got != tc.expected {
			t.Errorf("MaskPhone(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestMaskNote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Allergic to cats", "All***"},
		{"AB", "***"},
		{"", ""},
		{"Hello world", "Hel***"},
	}
	for _, tc := range tests {
		got := MaskNote(tc.input)
		if got != tc.expected {
			t.Errorf("MaskNote(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
