package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestFileStoreLoadReturnsErrNoTokenWhenAbsent(t *testing.T) {
	s := &fileStore{path: filepath.Join(t.TempDir(), "missing.json")}
	if _, err := s.Load(); !errors.Is(err, ErrNoToken) {
		t.Errorf("got %v, want ErrNoToken", err)
	}
}

func TestFileStoreSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &fileStore{path: filepath.Join(dir, "token.json")}

	want := &oauth2.Token{
		AccessToken:  "atk",
		RefreshToken: "rtk",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour).UTC().Truncate(time.Second),
	}
	if err := s.Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.AccessToken != want.AccessToken ||
		got.RefreshToken != want.RefreshToken ||
		got.TokenType != want.TokenType ||
		!got.Expiry.Equal(want.Expiry) {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestFileStoreSaveSetsRestrictedPermissions(t *testing.T) {
	dir := t.TempDir()
	s := &fileStore{path: filepath.Join(dir, "token.json")}
	if err := s.Save(&oauth2.Token{AccessToken: "x"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(s.path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("token file mode: got %o, want 0600", mode)
	}
}

func TestFileStoreSaveIsAtomic(t *testing.T) {
	// After a successful Save, the parent directory should contain only
	// the final token file — no leftover .tmp files.
	dir := t.TempDir()
	s := &fileStore{path: filepath.Join(dir, "token.json")}
	if err := s.Save(&oauth2.Token{AccessToken: "x"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "token.json" {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected only token.json in dir, got %v", names)
	}
}

func TestFileStoreSavePanicsOnNilToken(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when saving nil token")
		}
	}()
	s := &fileStore{path: filepath.Join(t.TempDir(), "token.json")}
	_ = s.Save(nil)
}

func TestFileStoreLoadNeverReturnsNilNil(t *testing.T) {
	// Postcondition: never returns (nil, nil) — either a token or an error.
	dir := t.TempDir()
	s := &fileStore{path: filepath.Join(dir, "token.json")}

	tok, err := s.Load()
	if tok == nil && err == nil {
		t.Error("Load returned (nil, nil) on missing file")
	}

	if err := s.Save(&oauth2.Token{AccessToken: "x"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	tok, err = s.Load()
	if tok == nil && err == nil {
		t.Error("Load returned (nil, nil) after Save")
	}
}

func TestConfigRejectsPlaceholderCredentials(t *testing.T) {
	// The committed credentials.json is a placeholder; Config must refuse
	// to return it so callers get a clear error rather than a confusing
	// 401 from Google later.
	if _, err := Config(); !errors.Is(err, ErrPlaceholderCredentials) {
		t.Errorf("Config(): got %v, want ErrPlaceholderCredentials", err)
	}
}
