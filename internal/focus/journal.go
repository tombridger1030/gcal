package focus

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Recorder is the narrow persistence interface the UI needs.
type Recorder interface {
	Append(Entry) error
}

// Journal stores focus entries as append-only JSON lines.
type Journal struct {
	path string
}

// DefaultJournal returns the focus journal under the platform user-config
// directory (on macOS: ~/Library/Application Support/gcal/focus.jsonl).
// The parent directory is created mode 0700.
func DefaultJournal() (*Journal, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate user config dir: %w", err)
	}
	dir := filepath.Join(base, "gcal")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	return &Journal{path: filepath.Join(dir, "focus.jsonl")}, nil
}

// Path returns the journal file path for diagnostics.
func (j *Journal) Path() string {
	if j == nil {
		return ""
	}
	return j.path
}

// Append writes e as one JSON line using O_APPEND|O_CREATE|O_WRONLY.
//
// Contract:
//
//	Preconditions: e.Rating must be in [1, 5].
//	Postcondition: on nil error, the file exists mode 0600 and contains
//	  exactly one additional JSON line for e.
func (j *Journal) Append(e Entry) error {
	if j == nil {
		panic("focus.Journal.Append: nil receiver")
	}
	if !ValidRating(e.Rating) {
		panic(fmt.Sprintf("focus.Journal.Append: rating %d outside 1..5", e.Rating))
	}
	if err := os.MkdirAll(filepath.Dir(j.path), 0o700); err != nil {
		return fmt.Errorf("create focus journal dir: %w", err)
	}
	f, err := os.OpenFile(j.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open focus journal: %w", err)
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod focus journal: %w", err)
	}

	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode focus entry: %w", err)
	}
	line = append(line, '\n')
	n, err := f.Write(line)
	if err != nil {
		return fmt.Errorf("append focus entry: %w", err)
	}
	if n != len(line) {
		return io.ErrShortWrite
	}
	return nil
}

// ReadAll returns every focus entry in journal order. A missing journal is
// an empty log, not an error.
func (j *Journal) ReadAll() ([]Entry, error) {
	if j == nil {
		panic("focus.Journal.ReadAll: nil receiver")
	}
	f, err := os.Open(j.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open focus journal: %w", err)
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("decode focus journal line %d: %w", line, err)
		}
		if !ValidRating(e.Rating) {
			return nil, fmt.Errorf("decode focus journal line %d: rating %d outside 1..5", line, e.Rating)
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read focus journal: %w", err)
	}
	return entries, nil
}
