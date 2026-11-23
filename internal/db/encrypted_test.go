package db

import (
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/brymastr/questrade-ynab/internal/tokencrypt"
)

func TestEncryptedStore(t *testing.T) {
	raw, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	key := make([]byte, 32)
	rand.Read(key)
	c, _ := tokencrypt.NewAES(base64.StdEncoding.EncodeToString(key))
	store := WithTokenEncryption(raw, c)

	user, _ := raw.CreateUser()

	// Written through the wrapper: ciphertext at rest, plaintext out.
	if err := store.UpsertToken(OAuthToken{UserID: user, Provider: "ynab", AccessToken: "acc", RefreshToken: "ref"}); err != nil {
		t.Fatal(err)
	}
	atRest, _ := raw.GetToken(user, "ynab")
	if !tokencrypt.IsEncrypted(atRest.AccessToken) || !tokencrypt.IsEncrypted(atRest.RefreshToken) {
		t.Fatalf("stored in plaintext: %+v", atRest)
	}
	got, err := store.GetToken(user, "ynab")
	if err != nil || got.AccessToken != "acc" || got.RefreshToken != "ref" {
		t.Fatalf("read back: %+v, %v", got, err)
	}

	// A legacy plaintext row decrypts as-is and is rewritten encrypted.
	if err := raw.UpsertToken(OAuthToken{UserID: user, Provider: "questrade", AccessToken: "old-acc", RefreshToken: "old-ref"}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetToken(user, "questrade")
	if err != nil || got.RefreshToken != "old-ref" {
		t.Fatalf("legacy read: %+v, %v", got, err)
	}
	atRest, _ = raw.GetToken(user, "questrade")
	if !tokencrypt.IsEncrypted(atRest.RefreshToken) {
		t.Errorf("legacy row not re-encrypted: %+v", atRest)
	}

	if tok, err := store.GetToken(user, "missing"); err != nil || tok != nil {
		t.Errorf("missing token: %+v, %v", tok, err)
	}
}
