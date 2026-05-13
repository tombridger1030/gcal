// Package ui implements the Bubble Tea program that renders today's
// calendar as a vertical column of time-blocks. Callers see one entry
// point: Run. Everything else — the model, message taxonomy, transition
// timer, and rendering — is unexported.
package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// Run launches the TUI and blocks until the user quits or ctx is cancelled.
//
// Contract:
//
//	Preconditions: ctx and fetcher must be non-nil.
//	Postcondition: the terminal is restored before return; any in-flight
//	fetch is cancelled by ctx and aborts cleanly.
func Run(ctx context.Context, fetcher Fetcher) error {
	if ctx == nil {
		panic("ui.Run: ctx is nil")
	}
	if fetcher == nil {
		panic("ui.Run: fetcher is nil")
	}

	m := newModel(nil)
	m.ctx = ctx
	m.fetcher = fetcher

	prog := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithAltScreen(),
	)
	_, err := prog.Run()
	return err
}
