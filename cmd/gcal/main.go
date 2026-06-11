// Command gcal is a Mac CLI tool that pins to the side of your screen and
// shows today's Google Calendar as a column of time-blocks. The "current"
// indicator advances at every event boundary; the day rolls over at local
// midnight. See the README for first-run setup.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tombridger1030/gcal/internal/auth"
	"github.com/tombridger1030/gcal/internal/calendar"
	"github.com/tombridger1030/gcal/internal/focus"
	"github.com/tombridger1030/gcal/internal/notify"
	"github.com/tombridger1030/gcal/internal/ui"
)

// version, commit, and date are stamped at link time by goreleaser via
// -ldflags. Left as defaults for go build / go run.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gcal:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("gcal", flag.ExitOnError)
	login := fs.Bool("login", false, "run the OAuth consent flow and save a token")
	logout := fs.Bool("logout", false, "delete the saved token")
	showVersion := fs.Bool("version", false, "print version and exit")
	noFocus := fs.Bool("no-focus", false, "disable hourly focus check-ins for this session")
	focusLog := fs.Bool("focus-log", false, "print the local focus journal and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Printf("gcal %s (%s, built %s)\n", version, commit, date)
		return nil
	}

	focusInterval, err := focusIntervalFromEnv()
	if err != nil {
		return err
	}

	if *focusLog {
		journal, err := focus.DefaultJournal()
		if err != nil {
			return err
		}
		entries, err := journal.ReadAll()
		if err != nil {
			return err
		}
		return writeFocusLog(os.Stdout, entries)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := auth.DefaultStore()
	if err != nil {
		return err
	}

	switch {
	case *logout:
		err := os.Remove(store.Path())
		if errors.Is(err, os.ErrNotExist) {
			return nil // already logged out — not an error
		}
		return err
	case *login:
		if err := auth.EnsureCredentials(os.Stdin, os.Stderr); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "gcal: launching OAuth consent flow...")
		return auth.RunFirstTimeFlow(ctx, store)
	}

	if err := auth.EnsureCredentials(os.Stdin, os.Stderr); err != nil {
		return err
	}

	client, err := calendar.New(ctx, store)
	if err != nil {
		if errors.Is(err, auth.ErrNoToken) {
			fmt.Fprintln(os.Stderr, "gcal: no token saved; running OAuth consent flow...")
			if err := auth.RunFirstTimeFlow(ctx, store); err != nil {
				return err
			}
			client, err = calendar.New(ctx, store)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	var focusOpts ui.FocusOptions
	if !*noFocus {
		journal, err := focus.DefaultJournal()
		if err != nil {
			return err
		}
		focusOpts = ui.FocusOptions{
			Enabled:  true,
			Recorder: journal,
			Notifier: notify.Osascript{},
			Interval: focusInterval,
		}
	}

	return ui.Run(ctx, client, ui.Options{Focus: focusOpts})
}

func focusIntervalFromEnv() (time.Duration, error) {
	raw := os.Getenv("GCAL_FOCUS_INTERVAL")
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse GCAL_FOCUS_INTERVAL: %w", err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("parse GCAL_FOCUS_INTERVAL: must be > 0, got %s", raw)
	}
	return d, nil
}

func writeFocusLog(w io.Writer, entries []focus.Entry) error {
	if len(entries) == 0 {
		_, err := fmt.Fprintln(w, "No focus entries.")
		return err
	}
	if _, err := fmt.Fprintln(w, "Logged At         Hour         Rating"); err != nil {
		return err
	}
	for _, e := range entries {
		loggedAt := e.LoggedAt.In(time.Local).Format("2006-01-02 15:04")
		hourStart := e.HourStart.In(time.Local)
		hour := fmt.Sprintf("%s-%s", hourStart.Format("15:04"), hourStart.Add(time.Hour).Format("15:04"))
		if _, err := fmt.Fprintf(w, "%s  %-11s  %d\n", loggedAt, hour, e.Rating); err != nil {
			return err
		}
	}
	return nil
}
