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

// FetchDay returns every event from the user's primary calendar that
// overlaps the local day containing day. Recurring events are expanded
// server-side; cancelled events and self-declined invites are dropped;
// all times are normalized to time.Local before return.
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
//	  - The slice is owned by the caller and may be mutated freely.
//
//	Errors:
//	  - Returns ErrTokenRevoked when Google rejects the credentials
//	    (HTTP 401/403 from the API or a 4xx from the token endpoint).
//	  - Returns wrapped network errors otherwise.
func (c *Client) FetchDay(ctx context.Context, day time.Time) ([]schedule.Event, error) {
	if day.IsZero() {
		panic("calendar.Client.FetchDay: day is zero")
	}
	day = day.Local()
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	dayEnd := time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, day.Location())

	call := c.svc.Events.List("primary").
		TimeMin(dayStart.Format(time.RFC3339)).
		TimeMax(dayEnd.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		MaxResults(2500).
		ShowDeleted(false).
		Context(ctx)

	resp, err := call.Do()
	if err != nil {
		return nil, classifyAPIError(err)
	}

	out := make([]schedule.Event, 0, len(resp.Items))
	for _, item := range resp.Items {
		ev, ok := translate(item)
		if !ok {
			continue
		}
		out = append(out, ev)
	}
	return out, nil
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
