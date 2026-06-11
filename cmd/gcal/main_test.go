package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/tombridger1030/gcal/internal/focus"
)

func TestFocusIntervalFromEnv(t *testing.T) {
	t.Setenv("GCAL_FOCUS_INTERVAL", "60s")
	got, err := focusIntervalFromEnv()
	if err != nil {
		t.Fatalf("focusIntervalFromEnv: %v", err)
	}
	if got != time.Minute {
		t.Errorf("duration=%v, want 1m", got)
	}
}

func TestWriteFocusLog(t *testing.T) {
	var buf bytes.Buffer
	entry := focus.Entry{
		HourStart: time.Date(2026, 5, 13, 13, 0, 0, 0, time.Local),
		Rating:    4,
		LoggedAt:  time.Date(2026, 5, 13, 14, 2, 0, 0, time.Local),
	}
	if err := writeFocusLog(&buf, []focus.Entry{entry}); err != nil {
		t.Fatalf("writeFocusLog: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Logged At", "2026-05-13 14:02", "13:00-14:00", "4"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestWriteFocusLogEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := writeFocusLog(&buf, nil); err != nil {
		t.Fatalf("writeFocusLog: %v", err)
	}
	if got := buf.String(); got != "No focus entries.\n" {
		t.Errorf("empty output=%q", got)
	}
}
