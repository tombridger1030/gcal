package auth

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// EnsureCredentials returns nil when usable OAuth credentials are already
// available — either on disk at <UserConfigDir>/gcal/credentials.json or
// embedded in the binary. When neither source is usable and stdin is a
// terminal, it walks the user through entering their Google Cloud OAuth
// Desktop client values, writes the result to the disk path, and returns
// nil. The actual consent flow (browser, code exchange, token save) is
// driven separately by RunFirstTimeFlow.
//
// Contract:
//
//	Preconditions:
//	  - in and out are non-nil. Stderr-style output (prompts, hints) is
//	    written to out; per-field input is read line-by-line from in.
//
//	Postconditions on nil error:
//	  - A subsequent call to Config() succeeds.
//	  - If a prompt occurred, the disk credentials file exists with
//	    mode 0600 and contains the entered client_id and client_secret.
//	  - The function performs no network IO and does not touch the
//	    refresh-token store.
//
//	Errors:
//	  - Returns ErrPlaceholderCredentials when credentials are missing
//	    and in is not a TTY (so a non-interactive caller still sees the
//	    same sentinel as before).
//	  - Returns wrapped parse / IO errors otherwise.
func EnsureCredentials(in io.Reader, out io.Writer) error {
	if in == nil || out == nil {
		panic("auth.EnsureCredentials: in and out must be non-nil")
	}
	switch _, err := Config(); {
	case err == nil:
		return nil
	case errors.Is(err, ErrPlaceholderCredentials):
		// fall through to prompt
	default:
		return err
	}

	if !isTerminal(in) {
		return ErrPlaceholderCredentials
	}

	fmt.Fprintln(out, "gcal: first-time setup — Google OAuth credentials.")
	fmt.Fprintln(out, "  Create (or open) a Desktop OAuth client at:")
	fmt.Fprintln(out, "    https://console.cloud.google.com/apis/credentials")
	fmt.Fprintln(out, "  See https://github.com/tombridger1030/gcal#first-run-setup for the full walkthrough.")
	fmt.Fprintln(out)

	r := bufio.NewReader(in)
	clientID, err := readRequired(r, out, "  Client ID:     ")
	if err != nil {
		return err
	}
	clientSecret, err := readRequired(r, out, "  Client secret: ")
	if err != nil {
		return err
	}
	projectID, err := readOptional(r, out, "  Project ID (optional, default \"gcal\"): ", "gcal")
	if err != nil {
		return err
	}

	data, err := buildCredentialsJSON(clientID, clientSecret, projectID)
	if err != nil {
		return err
	}

	path, err := credentialsPath()
	if err != nil {
		return fmt.Errorf("locate credentials path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := writeAtomic0600(path, data); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "  Saved credentials to", path)
	fmt.Fprintln(out)
	return nil
}

// readRequired prompts until it gets a non-empty line. EOF on first read
// returns ErrPlaceholderCredentials so the caller surfaces the same
// sentinel a non-TTY invocation would.
func readRequired(r *bufio.Reader, out io.Writer, label string) (string, error) {
	for attempt := 0; ; attempt++ {
		fmt.Fprint(out, label)
		line, err := r.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if errors.Is(err, io.EOF) && line == "" {
			return "", ErrPlaceholderCredentials
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read %s: %w", strings.TrimSpace(label), err)
		}
		if line != "" {
			return line, nil
		}
		fmt.Fprintln(out, "    (required — please try again)")
		if attempt >= 4 {
			return "", errors.New("too many empty attempts; aborting setup")
		}
	}
}

func readOptional(r *bufio.Reader, out io.Writer, label, dflt string) (string, error) {
	fmt.Fprint(out, label)
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read %s: %w", strings.TrimSpace(label), err)
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return dflt, nil
	}
	return line, nil
}

// buildCredentialsJSON produces a Google Cloud "installed app" credentials
// document with the entered values plus the canonical endpoint URLs Google
// publishes for the OAuth Desktop client type.
func buildCredentialsJSON(clientID, clientSecret, projectID string) ([]byte, error) {
	type installed struct {
		ClientID                string   `json:"client_id"`
		ProjectID               string   `json:"project_id"`
		AuthURI                 string   `json:"auth_uri"`
		TokenURI                string   `json:"token_uri"`
		AuthProviderX509CertURL string   `json:"auth_provider_x509_cert_url"`
		ClientSecret            string   `json:"client_secret"`
		RedirectURIs            []string `json:"redirect_uris"`
	}
	doc := struct {
		Installed installed `json:"installed"`
	}{
		Installed: installed{
			ClientID:                clientID,
			ProjectID:               projectID,
			AuthURI:                 "https://accounts.google.com/o/oauth2/auth",
			TokenURI:                "https://oauth2.googleapis.com/token",
			AuthProviderX509CertURL: "https://www.googleapis.com/oauth2/v1/certs",
			ClientSecret:            clientSecret,
			RedirectURIs:            []string{"http://localhost"},
		},
	}
	return json.MarshalIndent(doc, "", "  ")
}

// writeAtomic0600 mirrors the token-store atomic-write pattern: write to a
// sibling tmp file with mode 0600, then rename into place. Leaves no
// partial file on failure.
func writeAtomic0600(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// isTerminal reports whether r is a character device (i.e. a TTY).
// Indirected through a var so tests using *strings.Reader can still
// exercise the prompt path without crafting a real pty.
var isTerminal = func(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
