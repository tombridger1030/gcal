package ui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tombridger1030/gcal/internal/calendar"
	"github.com/tombridger1030/gcal/internal/schedule"
)

// Fetcher is the only thing the UI knows about the outside world. Tests
// supply a fake; production wires in *calendar.Client.
type Fetcher interface {
	FetchDay(ctx context.Context, day time.Time) ([]schedule.Event, error)
}

const (
	// timerCap bounds any single sleep so monotonic-clock skew during
	// macOS sleep cannot strand the UI on a stale block for hours.
	timerCap = 5 * time.Minute
	// wakeThreshold is the delay-past-scheduled that's interpreted as
	// "the host slept" and triggers a forced refresh.
	wakeThreshold = time.Minute
	// fetchTimeout is the per-call timeout for FetchDay.
	fetchTimeout = 15 * time.Second
)

// model is the Bubble Tea model. All fields are unexported. The only
// observable surface is Run() in run.go.
//
// Contract:
//
//	Invariants:
//	  - fetching is true exactly between dispatching a fetch command and
//	    receiving fetchDoneMsg or fetchErrMsg.
//	  - When revoked is true, no further fetch commands are dispatched.
//	  - state is always equal to schedule.BuildState(events, eventsAt).
type model struct {
	now func() time.Time

	ctx     context.Context
	fetcher Fetcher

	// width/height come from tea.WindowSizeMsg; rendered defensively
	// when zero (initial pre-resize frame).
	width, height int

	events   []schedule.Event
	eventsAt time.Time
	state    schedule.ScheduleState

	fetching bool
	stale    bool
	revoked  bool

	// scheduledAt is the wall-clock time at which the most recent
	// transition timer was *intended* to fire. Used for wake detection.
	scheduledAt time.Time
}

func newModel(now func() time.Time) model {
	if now == nil {
		now = time.Now
	}
	return model{now: now, ctx: context.Background()}
}

func (m *model) recomputeState() {
	m.state = schedule.BuildState(m.events, m.now())
}

// Init returns the boot command: fetch today's events.
func (m model) Init() tea.Cmd {
	return m.fetchCmd(m.now())
}

// Update is the tea.Model entry point; delegates to the testable update.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m.update(msg)
}

// View renders the current model state. The terminal-pre-resize case
// (width=0) renders nothing; the first WindowSizeMsg fills it in.
func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	return renderView(viewState{
		schedule: m.state,
		width:    m.width,
		height:   m.height,
		stale:    m.stale,
		revoked:  m.revoked,
	})
}

// update is the pure-ish state machine. Returns the next model and an
// optional command. All decisions live here; tests drive it directly.
func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			if m.fetching || m.revoked {
				return m, nil
			}
			m.fetching = true
			return m, m.fetchCmd(m.now())
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case fetchDoneMsg:
		m.events = msg.events
		m.eventsAt = msg.at
		m.fetching = false
		m.stale = false
		m.revoked = false
		m.recomputeState()
		return m, m.scheduleNextTransition()

	case fetchErrMsg:
		m.fetching = false
		if errors.Is(msg.err, calendar.ErrTokenRevoked) {
			m.revoked = true
			return m, nil
		}
		m.stale = true
		return m, m.scheduleNextTransition()

	case transitionMsg:
		now := m.now()
		dayRolledOver := now.After(startOfNextDay(m.state.Day)) ||
			now.Equal(startOfNextDay(m.state.Day))
		woke := !m.scheduledAt.IsZero() &&
			wakeDetected(m.scheduledAt, msg.at, wakeThreshold)
		m.recomputeState()

		if (dayRolledOver || woke) && !m.fetching && !m.revoked {
			m.fetching = true
			return m, tea.Batch(m.fetchCmd(now), m.scheduleNextTransition())
		}
		return m, m.scheduleNextTransition()
	}

	return m, nil
}

// startOfNextDay duplicates schedule's helper to avoid exporting it.
func startOfNextDay(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, day.Location())
}
