package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
)

// ErrNoToken is returned by TokenStore.Load when no token has been saved
// (i.e. the first-run flow has not completed).
var ErrNoToken = errors.New("no token saved; run `gcal --login` first")

// TokenStore persists the refresh token across runs. It is the only
// concept the rest of the program needs in order to talk about
// authentication state.
//
// Contract:
//
//	Load:
//	  Postcondition: returns either (nil, ErrNoToken) when no token has
//	  been saved, or (token, nil) where token is non-nil and previously
//	  passed to Save. Never returns (nil, nil).
//
//	Save:
//	  Precondition: t must be non-nil. Implementations may panic or
//	  return an error if violated.
//	  Postcondition: on nil error, a subsequent Load returns a token
//	  with the same AccessToken, RefreshToken, TokenType, and Expiry.
//	  The persisted artifact is readable only by the current user
//	  (file mode 0600 for file-backed implementations).
//
//	Path: returns a stable on-disk location used for diagnostics only.
//	  Callers must not read from or write to it directly.
type TokenStore interface {
	Load() (*oauth2.Token, error)
	Save(*oauth2.Token) error
	Path() string
}

// DefaultStore returns a TokenStore backed by a file under the platform
// user-config directory (on macOS: ~/Library/Application Support/gcal/).
// The directory is created with mode 0700; the token file is written with
// mode 0600 via atomic rename.
func DefaultStore() (TokenStore, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate user config dir: %w", err)
	}
	dir := filepath.Join(base, "gcal")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	return &fileStore{path: filepath.Join(dir, "token.json")}, nil
}

type fileStore struct {
	path string
}

func (s *fileStore) Path() string { return s.path }

func (s *fileStore) Load() (*oauth2.Token, error) {
	f, err := os.Open(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNoToken
	}
	if err != nil {
		return nil, fmt.Errorf("open token: %w", err)
	}
	defer f.Close()

	var t oauth2.Token
	if err := json.NewDecoder(f).Decode(&t); err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}
	return &t, nil
}

func (s *fileStore) Save(t *oauth2.Token) error {
	if t == nil {
		panic("auth.fileStore.Save: token is nil")
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".token-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp token: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("chmod temp token: %w", err)
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(t); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("encode token: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp token: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		cleanup()
		return fmt.Errorf("rename token: %w", err)
	}
	return nil
}
