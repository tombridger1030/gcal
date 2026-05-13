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
	// The committed credentials.json is a placeholder; with no disk override
	// Config must refuse to return it so callers get a clear error rather
	// than a confusing 401 from Google later.
	withCredentialsPath(t, filepath.Join(t.TempDir(), "missing.json"))
	if _, err := Config(); !errors.Is(err, ErrPlaceholderCredentials) {
		t.Errorf("Config(): got %v, want ErrPlaceholderCredentials", err)
	}
}

func TestConfigLoadsDiskCredentialsWhenPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	const realJSON = `{
		"installed": {
			"client_id": "real-client-id.apps.googleusercontent.com",
			"project_id": "real-project",
			"auth_uri": "https://accounts.google.com/o/oauth2/auth",
			"token_uri": "https://oauth2.googleapis.com/token",
			"client_secret": "real-secret",
			"redirect_uris": ["http://localhost"]
		}
	}`
	if err := os.WriteFile(path, []byte(realJSON), 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	withCredentialsPath(t, path)

	cfg, err := Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if cfg.ClientID != "real-client-id.apps.googleusercontent.com" {
		t.Errorf("ClientID: got %q, want disk client id", cfg.ClientID)
	}
}

func TestConfigRejectsDiskPlaceholder(t *testing.T) {
	// A disk credentials.json that still contains REPLACE_ME must produce
	// a clear error rather than silently falling through to the embedded
	// placeholder.
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	const placeholderJSON = `{
		"installed": {
			"client_id": "REPLACE_ME.apps.googleusercontent.com",
			"client_secret": "REPLACE_ME",
			"auth_uri": "https://accounts.google.com/o/oauth2/auth",
			"token_uri": "https://oauth2.googleapis.com/token",
			"redirect_uris": ["http://localhost"]
		}
	}`
	if err := os.WriteFile(path, []byte(placeholderJSON), 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	withCredentialsPath(t, path)

	if _, err := Config(); err == nil {
		t.Error("Config() with disk placeholder: got nil error, want placeholder rejection")
	}
}

// withCredentialsPath pins credentialsPath to p for the duration of the test
// and restores the original on cleanup.
func withCredentialsPath(t *testing.T, p string) {
	t.Helper()
	orig := credentialsPath
	credentialsPath = func() (string, error) { return p, nil }
	t.Cleanup(func() { credentialsPath = orig })
}
