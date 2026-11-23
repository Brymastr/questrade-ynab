package api

import (
	"path/filepath"
	"testing"

	"github.com/brymastr/questrade-ynab/internal/db"
)

func newTestStore(t *testing.T) db.Store {
	t.Helper()
	s, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestResolveQuestradeUser(t *testing.T) {
	store := newTestStore(t)

	// First connect with no session creates a linked user.
	alice, err := resolveQuestradeUser(store, "111", "")
	if err != nil || alice == "" {
		t.Fatalf("create: %q, %v", alice, err)
	}
	// Same login, lost cookie: same user back.
	if got, _ := resolveQuestradeUser(store, "111", ""); got != alice {
		t.Errorf("lost cookie: got %q, want %q", got, alice)
	}
	// Same login while holding some other session: still the linked user.
	if got, _ := resolveQuestradeUser(store, "111", "stale-session"); got != alice {
		t.Errorf("stale session: got %q, want %q", got, alice)
	}

	// A user created before linking existed gets adopted by their session.
	legacy, _ := store.CreateUser()
	if got, _ := resolveQuestradeUser(store, "222", legacy); got != legacy {
		t.Errorf("adopt legacy: got %q, want %q", got, legacy)
	}
	if qt, _ := store.QuestradeIDOf(legacy); qt != "222" {
		t.Errorf("legacy linked to %q, want 222", qt)
	}

	// A different login while logged in as alice gets its own user; alice
	// keeps her link.
	bob, _ := resolveQuestradeUser(store, "333", alice)
	if bob == "" || bob == alice {
		t.Errorf("second login reused alice's user: %q", bob)
	}
	if qt, _ := store.QuestradeIDOf(alice); qt != "111" {
		t.Errorf("alice relinked to %q", qt)
	}
}

func TestQuestradeUserAllowed(t *testing.T) {
	t.Setenv("ALLOWED_QUESTRADE_USER_IDS", "")
	if !questradeUserAllowed("999") {
		t.Error("empty allowlist should allow everyone")
	}
	t.Setenv("ALLOWED_QUESTRADE_USER_IDS", " 111 , 222")
	if !questradeUserAllowed("222") || questradeUserAllowed("999") {
		t.Error("allowlist not applied")
	}
}
