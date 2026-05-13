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

// ErrPlaceholderCredentials is returned when the embedded credentials.json
// has not been replaced with a real Google Cloud OAuth Desktop client
// credentials file. See README for setup instructions.
var ErrPlaceholderCredentials = errors.New(
	"credentials.json contains placeholder values; replace internal/auth/credentials.json with the credentials downloaded from your Google Cloud OAuth client and rebuild")

// Config returns the OAuth2 configuration parsed from the embedded
// credentials. Callers do not need to know the file format or the scope.
func Config() (*oauth2.Config, error) {
	cfg, err := google.ConfigFromJSON(credentialsJSON, calendarReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	if cfg.ClientID == "" || strings.Contains(cfg.ClientID, "REPLACE_ME") {
		return nil, ErrPlaceholderCredentials
	}
	return cfg, nil
}
