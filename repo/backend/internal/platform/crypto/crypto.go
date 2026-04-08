package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

// Key reads the ENCRYPTION_KEY env var (64 hex chars = 32 bytes = AES-256).
func Key() ([]byte, error) {
	hexKey := os.Getenv("ENCRYPTION_KEY")
	if hexKey == "" {
		return nil, errors.New("ENCRYPTION_KEY not set")
	}
	return hex.DecodeString(hexKey)
}

// Encrypt encrypts plaintext using AES-256-GCM and returns hex-encoded ciphertext.
func Encrypt(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// MaskPhone masks a phone number for display, showing only the last 4 digits.
// Example: "+1-555-867-5309" -> "***5309"
func MaskPhone(phone string) string {
	if phone == "" {
		return ""
	}
	// Extract only digits
	digits := ""
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits += string(r)
		}
	}
	if len(digits) <= 4 {
		return "***" + digits
	}
	return "***" + digits[len(digits)-4:]
}

// MaskNote masks a note string for display, showing only the first 3 characters
// followed by an ellipsis indicator.
func MaskNote(note string) string {
	if note == "" {
		return ""
	}
	if len(note) <= 3 {
		return "***"
	}
	return note[:3] + "***"
}

// Decrypt decrypts hex-encoded AES-256-GCM ciphertext and returns plaintext.
func Decrypt(key []byte, hexCiphertext string) (string, error) {
	ciphertext, err := hex.DecodeString(hexCiphertext)
	if err != nil {
		return "", fmt.Errorf("crypto: decode hex: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("crypto: ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decrypt: %w", err)
	}
	return string(plaintext), nil
}
