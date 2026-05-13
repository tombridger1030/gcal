package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	"golang.org/x/oauth2"
)

const successHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>gcal</title></head>
<body style="font-family:system-ui;text-align:center;margin-top:25vh">
<h2>gcal authorized</h2><p>You can close this tab.</p></body></html>`

// RunFirstTimeFlow drives the OAuth Desktop loopback flow:
//   - listens on a random localhost port,
//   - prints (and tries to open) the consent URL,
//   - exchanges the returned code (with PKCE) for a refresh token,
//   - saves the token via the store.
//
// It blocks until the flow completes, the context is cancelled, or an error
// occurs. The HTTP server is torn down before return regardless of outcome.
func RunFirstTimeFlow(ctx context.Context, store TokenStore) error {
	cfg, err := Config()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("bind loopback: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	cfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	verifier, err := randomURLSafe(64)
	if err != nil {
		_ = listener.Close()
		return err
	}
	state, err := randomURLSafe(24)
	if err != nil {
		_ = listener.Close()
		return err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	authURL := cfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	fmt.Println("Open this URL to authorize gcal:")
	fmt.Println()
	fmt.Println("  " + authURL)
	fmt.Println()
	openInBrowser(authURL)

	type result struct {
		token *oauth2.Token
		err   error
	}
	done := make(chan result, 1)
	// send delivers a result to done only if done is empty, so a duplicate
	// /callback (browser retry, prefetch) cannot wedge the handler goroutine
	// against a full buffer and stall server.Shutdown.
	send := func(r result) {
		select {
		case done <- r:
		default:
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("state"); got != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			send(result{err: errors.New("oauth state mismatch")})
			return
		}
		if oauthErr := q.Get("error"); oauthErr != "" {
			http.Error(w, oauthErr, http.StatusBadRequest)
			send(result{err: fmt.Errorf("oauth error: %s", oauthErr)})
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			send(result{err: errors.New("oauth callback missing code")})
			return
		}
		token, err := cfg.Exchange(r.Context(), code,
			oauth2.SetAuthURLParam("code_verifier", verifier),
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			send(result{err: fmt.Errorf("exchange code: %w", err)})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, successHTML)
		send(result{token: token})
	})

	server := &http.Server{Handler: mux}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	defer func() {
		shutdownCtx, cancel := context.WithCancel(context.Background())
		_ = server.Shutdown(shutdownCtx)
		cancel()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("loopback server: %w", err)
		}
		return errors.New("loopback server closed before callback")
	case r := <-done:
		if r.err != nil {
			return r.err
		}
		return store.Save(r.token)
	}
}

func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func openInBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	_ = cmd.Start()
}
