// Package tokencrypt encrypts OAuth tokens before they're stored, so reading the
// database alone doesn't reveal them.
//
// On AWS it uses KMS directly (tokens are far below the 4 KB limit), so only
// principals allowed to use the key can decrypt and every use is in CloudTrail.
// Locally it uses AES-256-GCM with a key from the environment.
//
// Each ciphertext is bound to its record (user ID and provider) through KMS
// encryption context or GCM additional data, so a ciphertext copied into
// another user's record fails to decrypt.
package tokencrypt

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// Binding identifies the record a token belongs to.
type Binding struct {
	UserID   string
	Provider string
}

func (b Binding) context() map[string]string {
	return map[string]string{"user_id": b.UserID, "provider": b.Provider}
}

func (b Binding) aad() []byte {
	return []byte("user_id=" + b.UserID + "\x00provider=" + b.Provider)
}

// Cipher encrypts and decrypts token strings. Ciphertexts carry a scheme
// prefix; Decrypt reports legacy=true for values stored before encryption
// existed (no prefix), returning them unchanged so callers can re-encrypt.
type Cipher interface {
	Encrypt(ctx context.Context, plaintext string, b Binding) (string, error)
	Decrypt(ctx context.Context, stored string, b Binding) (plaintext string, legacy bool, err error)
}

const (
	kmsPrefix = "kms1:"
	aesPrefix = "aes1:"
)

// IsEncrypted reports whether a stored value carries a ciphertext prefix.
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, kmsPrefix) || strings.HasPrefix(stored, aesPrefix)
}

// --- KMS ---

type kmsCipher struct {
	client *kms.Client
	keyID  string
}

func NewKMS(client *kms.Client, keyID string) Cipher {
	return &kmsCipher{client: client, keyID: keyID}
}

func (c *kmsCipher) Encrypt(ctx context.Context, plaintext string, b Binding) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	out, err := c.client.Encrypt(ctx, &kms.EncryptInput{
		KeyId:             aws.String(c.keyID),
		Plaintext:         []byte(plaintext),
		EncryptionContext: b.context(),
	})
	if err != nil {
		return "", fmt.Errorf("kms encrypt: %w", err)
	}
	return kmsPrefix + base64.StdEncoding.EncodeToString(out.CiphertextBlob), nil
}

func (c *kmsCipher) Decrypt(ctx context.Context, stored string, b Binding) (string, bool, error) {
	if stored == "" {
		return "", false, nil
	}
	if !strings.HasPrefix(stored, kmsPrefix) {
		if IsEncrypted(stored) {
			return "", false, errors.New("token was encrypted with a different scheme")
		}
		return stored, true, nil
	}
	blob, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, kmsPrefix))
	if err != nil {
		return "", false, fmt.Errorf("decode ciphertext: %w", err)
	}
	out, err := c.client.Decrypt(ctx, &kms.DecryptInput{
		KeyId:             aws.String(c.keyID),
		CiphertextBlob:    blob,
		EncryptionContext: b.context(),
	})
	if err != nil {
		return "", false, fmt.Errorf("kms decrypt: %w", err)
	}
	return string(out.Plaintext), false, nil
}

// --- AES-256-GCM (local) ---

type aesCipher struct {
	gcm cipher.AEAD
}

// NewAES builds a local cipher from a base64-encoded 32-byte key.
func NewAES(keyB64 string) (Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(keyB64))
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &aesCipher{gcm: gcm}, nil
}

func (c *aesCipher) Encrypt(_ context.Context, plaintext string, b Binding) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := c.gcm.Seal(nonce, nonce, []byte(plaintext), b.aad())
	return aesPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func (c *aesCipher) Decrypt(_ context.Context, stored string, b Binding) (string, bool, error) {
	if stored == "" {
		return "", false, nil
	}
	if !strings.HasPrefix(stored, aesPrefix) {
		if IsEncrypted(stored) {
			return "", false, errors.New("token was encrypted with a different scheme")
		}
		return stored, true, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, aesPrefix))
	if err != nil {
		return "", false, fmt.Errorf("decode ciphertext: %w", err)
	}
	ns := c.gcm.NonceSize()
	if len(raw) < ns {
		return "", false, errors.New("ciphertext too short")
	}
	plain, err := c.gcm.Open(nil, raw[:ns], raw[ns:], b.aad())
	if err != nil {
		return "", false, errors.New("decrypt failed (wrong key or record)")
	}
	return string(plain), false, nil
}
