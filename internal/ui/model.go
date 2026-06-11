package ui

import (
	"context"
	"errors"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tombridger1030/gcal/internal/calendar"
	"github.com/tombridger1030/gcal/internal/focus"
	"github.com/tombridger1030/gcal/internal/notify"
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
//	  - When focusEnabled is true, recorder/notifier are non-nil.
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

	focusEnabled  bool
	focusRecorder focus.Recorder
	focusNotifier notify.Notifier
	focusInterval time.Duration
	prompting     bool
	promptTarget  time.Time
	nextFocusAt   time.Time
	focusGen      int
	focusErr      error
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
	return tea.Batch(m.fetchCmd(m.now()), m.scheduleFocusPrompt())
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
		schedule:       m.state,
		width:          m.width,
		height:         m.height,
		stale:          m.stale,
		revoked:        m.revoked,
		focusPrompting: m.prompting,
		focusHourStart: focus.HourStartForPrompt(m.promptTarget),
		focusErr:       m.focusErr != nil,
	})
}

// update is the pure-ish state machine. Returns the next model and an
// optional command. All decisions live here; tests drive it directly.
func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		if m.prompting {
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "s", "esc":
				m.prompting = false
				m.setNextFocusPromptAt(m.now())
				return m, m.rescheduleFocusPrompt()
			case "1", "2", "3", "4", "5":
				rating, _ := strconv.Atoi(msg.String())
				entry := focus.Entry{
					HourStart: focus.HourStartForPrompt(m.promptTarget),
					Rating:    rating,
					LoggedAt:  m.now(),
				}
				m.prompting = false
				m.focusErr = nil
				m.setNextFocusPromptAt(m.now())
				return m, tea.Batch(m.recordFocusCmd(entry), m.rescheduleFocusPrompt())
			}
			return m, nil
		}

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
		return m, tea.Batch(m.scheduleNextTransition(), m.rescheduleFocusPrompt())

	case fetchErrMsg:
		m.fetching = false
		if errors.Is(msg.err, calendar.ErrTokenRevoked) {
			m.revoked = true
			return m, nil
		}
		m.stale = true
		return m, tea.Batch(m.scheduleNextTransition(), m.rescheduleFocusPrompt())

	case transitionMsg:
		now := m.now()
		dayRolledOver := now.After(startOfNextDay(m.state.Day)) ||
			now.Equal(startOfNextDay(m.state.Day))
		woke := !m.scheduledAt.IsZero() &&
			wakeDetected(m.scheduledAt, msg.at, wakeThreshold)
		m.recomputeState()

		if (dayRolledOver || woke) && !m.fetching && !m.revoked {
			m.fetching = true
			return m, tea.Batch(m.fetchCmd(now), m.scheduleNextTransition(), m.rescheduleFocusPrompt())
		}
		return m, tea.Batch(m.scheduleNextTransition(), m.rescheduleFocusPrompt())

	case focusTickMsg:
		return m.handleFocusTick(msg)

	case focusLoggedMsg:
		m.focusErr = nil
		return m, nil

	case focusErrMsg:
		m.focusErr = msg.err
		return m, nil
	}

	return m, nil
}

func (m model) handleFocusTick(msg focusTickMsg) (tea.Model, tea.Cmd) {
	if !m.focusEnabled {
		return m, nil
	}
	if msg.gen != m.focusGen {
		return m, nil
	}
	now := m.now()
	if m.nextFocusAt.IsZero() {
		if !m.setNextFocusPromptAt(now) {
			return m, nil
		}
		return m, m.rescheduleFocusPrompt()
	}
	if now.Before(m.nextFocusAt) {
		return m, m.rescheduleFocusPrompt()
	}

	target := m.nextFocusAt
	m.setNextFocusPromptAt(now)
	missed := msg.at.Sub(target) > wakeThreshold
	if missed || !focus.PromptCoversWorkBlock(target, m.state.Blocks) {
		return m, m.rescheduleFocusPrompt()
	}
	if m.prompting {
		m.promptTarget = target
		return m, tea.Batch(
			m.notifyCmd("gcal focus check-in", "How focused was the last work hour?"),
			m.rescheduleFocusPrompt(),
		)
	}

	m.prompting = true
	m.promptTarget = target
	return m, tea.Batch(
		m.notifyCmd("gcal focus check-in", "How focused was the last work hour?"),
		m.rescheduleFocusPrompt(),
	)
}

func (m model) nextFocusPromptAt(now time.Time) (time.Time, bool) {
	if m.focusInterval > 0 && focus.PromptCoversWorkBlock(now, m.state.Blocks) {
		return now.Add(m.focusInterval), true
	}
	return focus.NextPromptAt(now, m.state.Blocks)
}

func (m *model) setNextFocusPromptAt(now time.Time) bool {
	next, ok := m.nextFocusPromptAt(now)
	if !ok {
		m.nextFocusAt = time.Time{}
		return false
	}
	m.nextFocusAt = next
	return true
}

// startOfNextDay duplicates schedule's helper to avoid exporting it.
func startOfNextDay(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, day.Location())
}
