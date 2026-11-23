package tokencrypt

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func newTestAES(t *testing.T) Cipher {
	t.Helper()
	key := make([]byte, 32)
	rand.Read(key)
	c, err := NewAES(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAESRoundTripAndBinding(t *testing.T) {
	ctx := context.Background()
	c := newTestAES(t)
	alice := Binding{UserID: "alice", Provider: "ynab"}

	enc, err := c.Encrypt(ctx, "secret-token", alice)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, aesPrefix) || strings.Contains(enc, "secret-token") {
		t.Fatalf("not encrypted: %q", enc)
	}
	got, legacy, err := c.Decrypt(ctx, enc, alice)
	if err != nil || legacy || got != "secret-token" {
		t.Fatalf("round trip: %q legacy=%v err=%v", got, legacy, err)
	}

	// Moved to another user's or provider's record: must not decrypt.
	for _, b := range []Binding{{UserID: "bob", Provider: "ynab"}, {UserID: "alice", Provider: "questrade"}} {
		if _, _, err := c.Decrypt(ctx, enc, b); err == nil {
			t.Errorf("decrypted under %+v", b)
		}
	}
	// A different key can't read it.
	if _, _, err := newTestAES(t).Decrypt(ctx, enc, alice); err == nil {
		t.Error("decrypted with another key")
	}
}

func TestLegacyAndEmpty(t *testing.T) {
	ctx := context.Background()
	c := newTestAES(t)
	b := Binding{UserID: "u", Provider: "questrade"}

	got, legacy, err := c.Decrypt(ctx, "plain-old-token", b)
	if err != nil || !legacy || got != "plain-old-token" {
		t.Errorf("legacy: %q legacy=%v err=%v", got, legacy, err)
	}
	if enc, _ := c.Encrypt(ctx, "", b); enc != "" {
		t.Errorf("empty encrypted to %q", enc)
	}
	if _, _, err := c.Decrypt(ctx, kmsPrefix+"AAAA", b); err == nil {
		t.Error("AES cipher accepted a KMS ciphertext")
	}
}

func TestNewAESRejectsBadKeys(t *testing.T) {
	for _, k := range []string{"", "not-base64!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := NewAES(k); err == nil {
			t.Errorf("accepted key %q", k)
		}
	}
}
