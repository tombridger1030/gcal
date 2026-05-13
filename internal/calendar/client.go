// Package calendar wraps the Google Calendar API and translates its types
// into the domain model defined in internal/schedule. It is the only
// package in the program that imports google.golang.org/api/calendar/v3.
//
// Callers see a single object with a single method. Everything below —
// OAuth-wrapped HTTP client, recurring-event expansion, RFC3339 parsing,
// the all-day vs timed disambiguation, conversion to local time, and
// refresh-token persistence — is hidden behind that interface.
package calendar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"golang.org/x/oauth2"
	gcalendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/tombridger1030/gcal/internal/auth"
	"github.com/tombridger1030/gcal/internal/schedule"
)

// ErrTokenRevoked is returned when Google rejects the refresh token. The UI
// surfaces this as a re-auth prompt.
var ErrTokenRevoked = errors.New("calendar: token revoked or invalid; re-run `gcal --login`")

// Client fetches today's events from the user's primary Google Calendar.
type Client struct {
	svc   *gcalendar.Service
	store auth.TokenStore
}

// New constructs a Client backed by the token in store. It does not perform
// any network IO; the first network call is deferred to FetchDay.
func New(ctx context.Context, store auth.TokenStore) (*Client, error) {
	cfg, err := auth.Config()
	if err != nil {
		return nil, err
	}
	tok, err := store.Load()
	if err != nil {
		return nil, err
	}

	src := &savingSource{
		base:  cfg.TokenSource(ctx, tok),
		store: store,
		last:  tok,
	}

	httpClient := oauth2.NewClient(ctx, src)
	httpClient.Timeout = 15 * time.Second

	svc, err := gcalendar.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("init calendar service: %w", err)
	}
	return &Client{svc: svc, store: store}, nil
}

// FetchDay returns every event that overlaps the local day containing day,
// merged across every calendar the user has marked Selected in their Google
// Calendar UI (i.e. the calendars visible in the official client). Recurring
// events are expanded server-side; cancelled events and self-declined invites
// are dropped; all times are normalized to time.Local before return.
//
// Contract:
//
//	Preconditions:
//	  - ctx must be non-nil and not yet cancelled at call time.
//	  - day must be a non-zero time. Panics on zero time.
//
//	Postconditions on nil error:
//	  - Every returned event has Start and End in time.Local.
//	  - Every returned event overlaps [localMidnight(day), localMidnight(day)+24h).
//	  - Events are sorted by Start ascending.
//	  - The slice is owned by the caller and may be mutated freely.
//	  - Pagination is fully consumed for every queried calendar.
//
//	Errors:
//	  - Returns ErrTokenRevoked when Google rejects the credentials
//	    (HTTP 401/403 from the API or a 4xx from the token endpoint).
//	  - Returns the first wrapped network error encountered across the
//	    parallel per-calendar fetches; remaining fetches are cancelled.
func (c *Client) FetchDay(ctx context.Context, day time.Time) ([]schedule.Event, error) {
	if day.IsZero() {
		panic("calendar.Client.FetchDay: day is zero")
	}
	day = day.Local()
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	dayEnd := time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, day.Location())

	calIDs, err := c.selectedCalendarIDs(ctx)
	if err != nil {
		return nil, err
	}

	type fetchResult struct {
		events []schedule.Event
		err    error
	}
	results := make(chan fetchResult, len(calIDs))

	fetchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, id := range calIDs {
		go func(id string) {
			evs, err := c.fetchDayForCalendar(fetchCtx, id, dayStart, dayEnd)
			results <- fetchResult{events: evs, err: err}
		}(id)
	}

	var (
		out      []schedule.Event
		firstErr error
	)
	for range calIDs {
		r := <-results
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
				cancel() // signal remaining goroutines to abort early
			}
			continue
		}
		out = append(out, r.events...)
	}
	if firstErr != nil {
		return nil, firstErr
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Start.Before(out[j].Start)
	})
	return out, nil
}

// selectedCalendarIDs returns the IDs of every calendar the user has marked
// visible in their Google Calendar UI. The primary calendar is always
// included even if the API ever fails to flag it.
func (c *Client) selectedCalendarIDs(ctx context.Context) ([]string, error) {
	var ids []string
	pageToken := ""
	sawPrimary := false
	for {
		call := c.svc.CalendarList.List().
			MinAccessRole("reader").
			ShowHidden(false).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, classifyAPIError(err)
		}
		for _, item := range resp.Items {
			if !item.Selected {
				continue
			}
			ids = append(ids, item.Id)
			if item.Primary {
				sawPrimary = true
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	if !sawPrimary {
		ids = append(ids, "primary")
	}
	return ids, nil
}

// fetchDayForCalendar fetches all events for one calendar across the given
// half-open [dayStart, dayEnd) window, consuming pagination fully.
func (c *Client) fetchDayForCalendar(ctx context.Context, calID string, dayStart, dayEnd time.Time) ([]schedule.Event, error) {
	var out []schedule.Event
	pageToken := ""
	for {
		call := c.svc.Events.List(calID).
			TimeMin(dayStart.Format(time.RFC3339)).
			TimeMax(dayEnd.Format(time.RFC3339)).
			SingleEvents(true).
			OrderBy("startTime").
			MaxResults(2500).
			ShowDeleted(false).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, classifyAPIError(err)
		}

		if out == nil {
			out = make([]schedule.Event, 0, len(resp.Items))
		}
		for _, item := range resp.Items {
			ev, ok := translate(item)
			if !ok {
				continue
			}
			out = append(out, ev)
		}

		if resp.NextPageToken == "" {
			return out, nil
		}
		pageToken = resp.NextPageToken
	}
}

// classifyAPIError converts Google's typed errors into the package's
// public sentinels where applicable.
func classifyAPIError(err error) error {
	var ge *googleapi.Error
	if errors.As(err, &ge) && (ge.Code == http.StatusUnauthorized || ge.Code == http.StatusForbidden) {
		return ErrTokenRevoked
	}
	var re *oauth2.RetrieveError
	if errors.As(err, &re) && re.Response != nil &&
		re.Response.StatusCode >= 400 && re.Response.StatusCode < 500 {
		return ErrTokenRevoked
	}
	return fmt.Errorf("calendar fetch: %w", err)
}

// savingSource persists the token to the store whenever oauth2 refreshes
// it, so a rotated refresh token survives across runs.
type savingSource struct {
	mu    sync.Mutex
	base  oauth2.TokenSource
	store auth.TokenStore
	last  *oauth2.Token
}

func (s *savingSource) Token() (*oauth2.Token, error) {
	tok, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	changed := s.last == nil ||
		tok.AccessToken != s.last.AccessToken ||
		tok.RefreshToken != s.last.RefreshToken
	if changed {
		s.last = tok
	}
	s.mu.Unlock()
	if changed {
		// Save best-effort; a save failure must not break the request.
		_ = s.store.Save(tok)
	}
	return tok, nil
}
