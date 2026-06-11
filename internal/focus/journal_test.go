package focus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testJournal(t *testing.T) *Journal {
	t.Helper()
	return &Journal{path: filepath.Join(t.TempDir(), "focus.jsonl")}
}

func TestJournalAppendReadAllRoundTrip(t *testing.T) {
	j := testJournal(t)
	entry := Entry{
		HourStart: focusAt(13, 0),
		Rating:    3,
		LoggedAt:  focusAt(14, 2),
	}
	if err := j.Append(entry); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := j.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entries len=%d, want 1", len(got))
	}
	if !got[0].HourStart.Equal(entry.HourStart) ||
		got[0].Rating != entry.Rating ||
		!got[0].LoggedAt.Equal(entry.LoggedAt) {
		t.Errorf("round trip mismatch: got %+v want %+v", got[0], entry)
	}
}

func TestJournalAppendAddsOneLinePerEntry(t *testing.T) {
	j := testJournal(t)
	for i := 1; i <= 2; i++ {
		if err := j.Append(Entry{HourStart: focusAt(9+i, 0), Rating: i, LoggedAt: time.Now()}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	raw, err := os.ReadFile(j.Path())
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if lines := strings.Count(string(raw), "\n"); lines != 2 {
		t.Errorf("newline count=%d, want 2; raw=%q", lines, string(raw))
	}
}

func TestJournalAppendSetsRestrictedPermissions(t *testing.T) {
	j := testJournal(t)
	if err := j.Append(Entry{HourStart: focusAt(9, 0), Rating: 5, LoggedAt: time.Now()}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	info, err := os.Stat(j.Path())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("journal mode=%o, want 0600", mode)
	}
}

func TestJournalReadAllMissingFileIsEmptyLog(t *testing.T) {
	j := testJournal(t)
	entries, err := j.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries len=%d, want 0", len(entries))
	}
}

func TestJournalAppendPanicsForInvalidRating(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid rating")
		}
	}()
	_ = testJournal(t).Append(Entry{HourStart: focusAt(9, 0), Rating: 6, LoggedAt: time.Now()})
}

func TestJournalReadAllRejectsInvalidJSONLine(t *testing.T) {
	j := testJournal(t)
	if err := os.WriteFile(j.Path(), []byte("{bad json}\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := j.ReadAll()
	if err == nil {
		t.Fatal("ReadAll err=nil, want decode error")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Errorf("error should include line number, got %v", err)
	}
}
