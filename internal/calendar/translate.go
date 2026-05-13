package calendar

import (
	"time"

	gcalendar "google.golang.org/api/calendar/v3"

	"github.com/tombridger1030/gcal/internal/schedule"
)

// translate converts one Google calendar.Event into the domain Event.
// It returns ok=false for events the UI shouldn't see (cancelled, declined
// invitations, malformed time fields).
func translate(item *gcalendar.Event) (schedule.Event, bool) {
	if item == nil || item.Status == "cancelled" {
		return schedule.Event{}, false
	}
	if isDeclined(item) {
		return schedule.Event{}, false
	}
	if item.Start == nil || item.End == nil {
		return schedule.Event{}, false
	}

	start, end, allDay, ok := parseTimes(item.Start, item.End)
	if !ok {
		return schedule.Event{}, false
	}

	title := item.Summary
	if title == "" {
		title = "(no title)"
	}

	return schedule.Event{
		ID:       item.Id,
		Title:    title,
		Start:    start.Local(),
		End:      end.Local(),
		AllDay:   allDay,
		Location: item.Location,
	}, true
}

func isDeclined(item *gcalendar.Event) bool {
	for _, a := range item.Attendees {
		if a.Self && a.ResponseStatus == "declined" {
			return true
		}
	}
	return false
}

// parseTimes handles Google's two encodings:
//   - timed events use Start.DateTime (RFC3339, often with offset)
//   - all-day events use Start.Date ("YYYY-MM-DD") and the End.Date is the
//     exclusive day after the last covered day.
func parseTimes(start, end *gcalendar.EventDateTime) (s, e time.Time, allDay bool, ok bool) {
	switch {
	case start.DateTime != "" && end.DateTime != "":
		var err error
		s, err = time.Parse(time.RFC3339, start.DateTime)
		if err != nil {
			return time.Time{}, time.Time{}, false, false
		}
		e, err = time.Parse(time.RFC3339, end.DateTime)
		if err != nil {
			return time.Time{}, time.Time{}, false, false
		}
		return s, e, false, true
	case start.Date != "" && end.Date != "":
		var err error
		s, err = time.ParseInLocation("2006-01-02", start.Date, time.Local)
		if err != nil {
			return time.Time{}, time.Time{}, false, false
		}
		e, err = time.ParseInLocation("2006-01-02", end.Date, time.Local)
		if err != nil {
			return time.Time{}, time.Time{}, false, false
		}
		return s, e, true, true
	default:
		return time.Time{}, time.Time{}, false, false
	}
}
