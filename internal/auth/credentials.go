// Package auth handles Google OAuth credentials, the first-run consent flow,
// and persistence of the resulting refresh token. The rest of the program
// only sees a TokenStore and the ability to invoke the first-run flow; the
// HTTP loopback server, PKCE handling, and file permissions live behind
// these two concepts.
package auth

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// calendarReadonlyScope is duplicated as a string literal rather than
// imported from google.golang.org/api/calendar/v3 to keep this package free
// of any Google API client dependency.
const calendarReadonlyScope = "https://www.googleapis.com/auth/calendar.readonly"

//go:embed credentials.json
var credentialsJSON []byte

// ErrPlaceholderCredentials is returned when neither the on-disk
// credentials.json nor the embedded one contain a real Google Cloud OAuth
// Desktop client. See README for setup instructions.
var ErrPlaceholderCredentials = errors.New(
	"no OAuth credentials found; drop your Google Cloud OAuth client JSON at <UserConfigDir>/gcal/credentials.json (on macOS: ~/Library/Application Support/gcal/credentials.json) — see README")

// Config returns the OAuth2 configuration.
//
// Lookup order:
//  1. A file at <UserConfigDir>/gcal/credentials.json — for end users who
//     install a prebuilt binary and drop their own client JSON next to the
//     token file.
//  2. The embedded credentials.json — for developers who replace it in the
//     repo and rebuild from source.
//
// Returns ErrPlaceholderCredentials when both sources are missing or still
// contain the REPLACE_ME placeholder.
func Config() (*oauth2.Config, error) {
	if disk, err := loadDiskCredentials(); err != nil {
		return nil, err
	} else if disk != nil {
		return disk, nil
	}

	cfg, err := google.ConfigFromJSON(credentialsJSON, calendarReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parse embedded credentials: %w", err)
	}
	if cfg.ClientID == "" || strings.Contains(cfg.ClientID, "REPLACE_ME") {
		return nil, ErrPlaceholderCredentials
	}
	return cfg, nil
}

// loadDiskCredentials returns the parsed Config when a non-placeholder
// credentials.json exists in the user config directory. (nil, nil) means
// the file does not exist — the embedded path should be tried.
func loadDiskCredentials() (*oauth2.Config, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, nil // no user config dir at all; fall back to embedded
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	cfg, err := google.ConfigFromJSON(data, calendarReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.ClientID == "" || strings.Contains(cfg.ClientID, "REPLACE_ME") {
		return nil, fmt.Errorf("%s contains placeholder values; replace it with your real OAuth client JSON", path)
	}
	return cfg, nil
}

// credentialsPath returns the on-disk path the user can drop OAuth client
// credentials into, kept next to the token file for symmetry. Indirected
// through a package-level var so tests can pin it to a TempDir.
var credentialsPath = func() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gcal", "credentials.json"), nil
}
