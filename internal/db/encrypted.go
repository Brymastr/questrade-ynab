package db

import (
	"context"
	"fmt"
	"log"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/brymastr/questrade-ynab/internal/tokencrypt"
)

// encryptedStore wraps a Store so OAuth tokens are encrypted before they reach
// the database and decrypted on the way out. Everything else passes through.
type encryptedStore struct {
	Store
	c tokencrypt.Cipher
}

// WithTokenEncryption wraps s so tokens are stored encrypted with c.
func WithTokenEncryption(s Store, c tokencrypt.Cipher) Store {
	return &encryptedStore{Store: s, c: c}
}

// tokenCipherFromEnv picks the cipher: TOKEN_KMS_KEY_ID (AWS) or
// TOKEN_ENCRYPTION_KEY (local, base64 32 bytes). One is required.
func tokenCipherFromEnv(ctx context.Context) (tokencrypt.Cipher, error) {
	if keyID := os.Getenv("TOKEN_KMS_KEY_ID"); keyID != "" {
		cfg, err := awsconfig.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("load aws config: %w", err)
		}
		return tokencrypt.NewKMS(kms.NewFromConfig(cfg), keyID), nil
	}
	if key := os.Getenv("TOKEN_ENCRYPTION_KEY"); key != "" {
		c, err := tokencrypt.NewAES(key)
		if err != nil {
			return nil, fmt.Errorf("TOKEN_ENCRYPTION_KEY: %w", err)
		}
		return c, nil
	}
	return nil, fmt.Errorf("set TOKEN_ENCRYPTION_KEY (generate with: openssl rand -base64 32) or TOKEN_KMS_KEY_ID")
}

func (e *encryptedStore) UpsertToken(t OAuthToken) error {
	ctx := context.Background()
	b := tokencrypt.Binding{UserID: t.UserID, Provider: t.Provider}
	var err error
	if t.AccessToken, err = e.c.Encrypt(ctx, t.AccessToken, b); err != nil {
		return err
	}
	if t.RefreshToken, err = e.c.Encrypt(ctx, t.RefreshToken, b); err != nil {
		return err
	}
	return e.Store.UpsertToken(t)
}

func (e *encryptedStore) GetToken(userID, provider string) (*OAuthToken, error) {
	t, err := e.Store.GetToken(userID, provider)
	if err != nil || t == nil {
		return t, err
	}
	ctx := context.Background()
	b := tokencrypt.Binding{UserID: userID, Provider: provider}
	access, legacyA, err := e.c.Decrypt(ctx, t.AccessToken, b)
	if err != nil {
		return nil, fmt.Errorf("decrypt %s access token: %w", provider, err)
	}
	refresh, legacyR, err := e.c.Decrypt(ctx, t.RefreshToken, b)
	if err != nil {
		return nil, fmt.Errorf("decrypt %s refresh token: %w", provider, err)
	}
	t.AccessToken, t.RefreshToken = access, refresh

	// Stored before encryption existed: rewrite it encrypted.
	if legacyA || legacyR {
		if err := e.UpsertToken(*t); err != nil {
			log.Printf("db: re-encrypting %s token for user %s failed: %v", provider, userID, err)
		}
	}
	return t, nil
}
