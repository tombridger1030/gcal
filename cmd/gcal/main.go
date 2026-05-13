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
	"os"
	"os/signal"
	"syscall"

	"github.com/tombridger1030/gcal/internal/auth"
	"github.com/tombridger1030/gcal/internal/calendar"
	"github.com/tombridger1030/gcal/internal/ui"
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
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := auth.DefaultStore()
	if err != nil {
		return err
	}

	switch {
	case *logout:
		return os.Remove(store.Path())
	case *login:
		fmt.Fprintln(os.Stderr, "gcal: launching OAuth consent flow...")
		return auth.RunFirstTimeFlow(ctx, store)
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

	return ui.Run(ctx, client)
}
