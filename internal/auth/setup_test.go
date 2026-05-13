package auth

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pinTerminal makes isTerminal return true for the duration of a test so
// EnsureCredentials enters the prompt branch with a non-*os.File reader.
func pinTerminal(t *testing.T) {
	t.Helper()
	orig := isTerminal
	isTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() { isTerminal = orig })
}

// EnsureCredentials writes a usable credentials.json to the pinned path
// when stdin supplies a complete answer set, and Config() then loads it.
func TestEnsureCredentialsPromptsAndPersists(t *testing.T) {
	pinTerminal(t)
	dir := t.TempDir()
	withCredentialsPath(t, filepath.Join(dir, "credentials.json"))

	in := strings.NewReader("real-client-id.apps.googleusercontent.com\nreal-secret\n\n")
	var out bytes.Buffer

	if err := EnsureCredentials(in, &out); err != nil {
		t.Fatalf("EnsureCredentials: %v", err)
	}

	if !strings.Contains(out.String(), "Client ID:") || !strings.Contains(out.String(), "Saved credentials to") {
		t.Errorf("expected prompt + save confirmation in output:\n%s", out.String())
	}

	// Subsequent Config() must succeed and reflect the entered values.
	cfg, err := Config()
	if err != nil {
		t.Fatalf("Config after EnsureCredentials: %v", err)
	}
	if cfg.ClientID != "real-client-id.apps.googleusercontent.com" {
		t.Errorf("ClientID: got %q, want entered value", cfg.ClientID)
	}
	if cfg.ClientSecret != "real-secret" {
		t.Errorf("ClientSecret: got %q, want entered value", cfg.ClientSecret)
	}

	// File must be 0600.
	path, _ := credentialsPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("credentials mode: got %o, want 0600", mode)
	}
}

// EnsureCredentials short-circuits with nil when disk credentials already
// exist — no prompt should be emitted.
func TestEnsureCredentialsNoOpWhenDiskCredsPresent(t *testing.T) {
	pinTerminal(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	withCredentialsPath(t, path)

	const valid = `{"installed":{"client_id":"prior.apps.googleusercontent.com","client_secret":"prior-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token","redirect_uris":["http://localhost"]}}`
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatalf("write prior creds: %v", err)
	}

	in := strings.NewReader("should-not-be-read\n")
	var out bytes.Buffer
	if err := EnsureCredentials(in, &out); err != nil {
		t.Fatalf("EnsureCredentials: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected no output when creds present, got:\n%s", out.String())
	}
}

// Non-TTY stdin with no creds returns ErrPlaceholderCredentials rather
// than hanging on stdin read.
func TestEnsureCredentialsReturnsErrWhenNotTTY(t *testing.T) {
	// Default isTerminal returns false for non-*os.File readers.
	dir := t.TempDir()
	withCredentialsPath(t, filepath.Join(dir, "missing.json"))

	in := strings.NewReader("")
	var out bytes.Buffer
	err := EnsureCredentials(in, &out)
	if !errors.Is(err, ErrPlaceholderCredentials) {
		t.Errorf("got %v, want ErrPlaceholderCredentials", err)
	}
	if out.Len() != 0 {
		t.Errorf("non-tty should not prompt; got output:\n%s", out.String())
	}
}

// EOF on the first required prompt also yields ErrPlaceholderCredentials —
// keeps the sentinel consistent whether the caller is a non-TTY pipe or
// a user who Ctrl-D'd at the first question.
func TestEnsureCredentialsEOFOnFirstFieldReturnsPlaceholderErr(t *testing.T) {
	pinTerminal(t)
	dir := t.TempDir()
	withCredentialsPath(t, filepath.Join(dir, "credentials.json"))

	in := strings.NewReader("") // immediate EOF
	var out bytes.Buffer
	err := EnsureCredentials(in, &out)
	if !errors.Is(err, ErrPlaceholderCredentials) {
		t.Errorf("got %v, want ErrPlaceholderCredentials", err)
	}
}

// An empty project ID falls back to "gcal".
func TestEnsureCredentialsProjectIDDefault(t *testing.T) {
	pinTerminal(t)
	dir := t.TempDir()
	withCredentialsPath(t, filepath.Join(dir, "credentials.json"))

	in := strings.NewReader("cid.apps.googleusercontent.com\nsecret\n\n")
	var out bytes.Buffer
	if err := EnsureCredentials(in, &out); err != nil {
		t.Fatalf("EnsureCredentials: %v", err)
	}

	path, _ := credentialsPath()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read creds: %v", err)
	}
	if !strings.Contains(string(body), `"project_id": "gcal"`) {
		t.Errorf("expected default project_id=gcal in saved JSON:\n%s", body)
	}
}
