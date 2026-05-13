package calendar

import (
	"testing"
	"time"

	gcalendar "google.golang.org/api/calendar/v3"
)

func TestTranslateTimedEvent(t *testing.T) {
	in := &gcalendar.Event{
		Id:      "abc",
		Summary: "Design review",
		Start:   &gcalendar.EventDateTime{DateTime: "2026-05-13T10:30:00-07:00"},
		End:     &gcalendar.EventDateTime{DateTime: "2026-05-13T11:30:00-07:00"},
	}
	got, ok := translate(in)
	if !ok {
		t.Fatal("translate returned ok=false for valid event")
	}
	if got.ID != "abc" || got.Title != "Design review" || got.AllDay {
		t.Errorf("fields: %+v", got)
	}
	if got.Start.Hour() != got.Start.Local().Hour() {
		t.Error("start not normalized to local time")
	}
}

func TestTranslateAllDayEvent(t *testing.T) {
	in := &gcalendar.Event{
		Id:      "ooo",
		Summary: "OOO",
		Start:   &gcalendar.EventDateTime{Date: "2026-05-13"},
		End:     &gcalendar.EventDateTime{Date: "2026-05-14"},
	}
	got, ok := translate(in)
	if !ok || !got.AllDay {
		t.Fatalf("expected all-day, got %+v ok=%v", got, ok)
	}
	wantStart := time.Date(2026, 5, 13, 0, 0, 0, 0, time.Local)
	wantEnd := time.Date(2026, 5, 14, 0, 0, 0, 0, time.Local)
	if !got.Start.Equal(wantStart) || !got.End.Equal(wantEnd) {
		t.Errorf("times: got [%v,%v], want [%v,%v]", got.Start, got.End, wantStart, wantEnd)
	}
}

func TestTranslateDropsCancelled(t *testing.T) {
	in := &gcalendar.Event{
		Id:     "x",
		Status: "cancelled",
		Start:  &gcalendar.EventDateTime{DateTime: "2026-05-13T10:30:00Z"},
		End:    &gcalendar.EventDateTime{DateTime: "2026-05-13T11:30:00Z"},
	}
	if _, ok := translate(in); ok {
		t.Error("expected cancelled event to be dropped")
	}
}

func TestTranslateDropsDeclinedByMe(t *testing.T) {
	in := &gcalendar.Event{
		Id:    "x",
		Start: &gcalendar.EventDateTime{DateTime: "2026-05-13T10:30:00Z"},
		End:   &gcalendar.EventDateTime{DateTime: "2026-05-13T11:30:00Z"},
		Attendees: []*gcalendar.EventAttendee{
			{Self: true, ResponseStatus: "declined"},
			{Self: false, ResponseStatus: "accepted"},
		},
	}
	if _, ok := translate(in); ok {
		t.Error("expected declined event to be dropped")
	}
}

func TestTranslateKeepsAcceptedInvite(t *testing.T) {
	in := &gcalendar.Event{
		Id:      "x",
		Summary: "Standup",
		Start:   &gcalendar.EventDateTime{DateTime: "2026-05-13T09:00:00Z"},
		End:     &gcalendar.EventDateTime{DateTime: "2026-05-13T09:30:00Z"},
		Attendees: []*gcalendar.EventAttendee{
			{Self: true, ResponseStatus: "accepted"},
		},
	}
	if _, ok := translate(in); !ok {
		t.Error("expected accepted event to be kept")
	}
}

func TestTranslateUsesPlaceholderForUntitled(t *testing.T) {
	in := &gcalendar.Event{
		Id:    "x",
		Start: &gcalendar.EventDateTime{DateTime: "2026-05-13T09:00:00Z"},
		End:   &gcalendar.EventDateTime{DateTime: "2026-05-13T09:30:00Z"},
	}
	got, ok := translate(in)
	if !ok || got.Title != "(no title)" {
		t.Errorf("title: got %q ok=%v, want (no title) ok=true", got.Title, ok)
	}
}

func TestTranslateRejectsMalformedTimes(t *testing.T) {
	in := &gcalendar.Event{
		Id:    "x",
		Start: &gcalendar.EventDateTime{DateTime: "not-a-time"},
		End:   &gcalendar.EventDateTime{DateTime: "also-not"},
	}
	if _, ok := translate(in); ok {
		t.Error("expected malformed time to be dropped")
	}
}
