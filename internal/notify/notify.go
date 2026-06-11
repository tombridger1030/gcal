// Package notify is the anti-corruption layer around macOS notifications.
package notify

import (
	"fmt"
	"os/exec"
	"strings"
)

// Notifier is the only notification concept the UI depends on.
type Notifier interface {
	Send(title, body string) error
}

// Osascript sends native macOS notifications using the built-in osascript
// command. Callers treat errors as best-effort notification failures.
type Osascript struct{}

// Send asks macOS to display a notification.
func (Osascript) Send(title, body string) error {
	cmd := exec.Command("osascript", "-e", buildScript(title, body))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("send macOS notification: %w", err)
	}
	return nil
}

func buildScript(title, body string) string {
	return fmt.Sprintf(
		"display notification \"%s\" with title \"%s\"",
		escapeString(body),
		escapeString(title),
	)
}

func escapeString(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
	)
	return replacer.Replace(s)
}
