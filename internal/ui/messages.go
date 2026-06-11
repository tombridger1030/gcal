package ui

import (
	"time"

	"github.com/tombridger1030/gcal/internal/schedule"
)

// fetchDoneMsg is the result of a successful FetchDay. The model replaces
// its event list and clears stale/revoked flags.
type fetchDoneMsg struct {
	events []schedule.Event
	at     time.Time
}

// fetchErrMsg signals a failed FetchDay. When stale is true, the prior
// event list is kept and the UI shows a stale indicator. An err matching
// calendar.ErrTokenRevoked transitions the model to the revoked state and
// stops scheduling further fetches.
type fetchErrMsg struct {
	err   error
	stale bool
}

// transitionMsg fires when the boundary timer elapses. The model
// recomputes status classification and reschedules.
type transitionMsg struct {
	at time.Time
}

// focusTickMsg fires when the focus prompt timer wakes.
type focusTickMsg struct {
	at  time.Time
	gen int
}

// focusLoggedMsg confirms a focus entry was appended.
type focusLoggedMsg struct{}

// focusErrMsg reports a failed focus journal write.
type focusErrMsg struct {
	err error
}
