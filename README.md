# gcal

A tiny terminal calendar that pins to the side of your Mac screen and shows
today's Google Calendar as a column of time-blocks. The "current" indicator
auto-advances at every event boundary; the day rolls over at local midnight.
No clock display, no daemon, no polling — pure event-driven rendering.

Events are merged from every calendar you have marked **visible** in the
Google Calendar UI (work + personal + shared, etc.), not just `primary`.

```
┌──────────────────────────────┐
│ Tue · May 13                 │
│ ──────────────────────────── │
│ all-day: OOO – Jamie         │
│                              │
│   09:00 – 09:30              │
│   standup                    │
│                              │
│ ▌ 10:30 – 11:30   ◀ NOW      │
│ ▌ Design review              │
│ ▌ Zoom                       │
│                              │
│   12:00 – 13:00              │
│   lunch w/ Sam               │
│   Tartine                    │
└──────────────────────────────┘
```

## Design

Five small Go packages, each a "deep module" (Ousterhout): narrow public
interface, substantial hidden implementation.

| Package             | Public surface                   | Hides                                                            |
| ------------------- | -------------------------------- | ---------------------------------------------------------------- |
| `internal/schedule` | `BuildState`, `NextTransition`   | sort, clamp, classify, midnight math, DST                        |
| `internal/calendar` | `New`, `FetchDay`                | Google SDK, RFC3339, recurring expansion, declined-invite filter |
| `internal/auth`     | `TokenStore`, `RunFirstTimeFlow` | OAuth loopback server, PKCE, atomic 0600 writes                  |
| `internal/ui`       | `Run`                            | Bubble Tea model, transition timer, render layout                |
| `cmd/gcal`          | `main`                           | flag parsing, wiring                                             |

Function preconditions and postconditions are documented as Design-by-Contract
blocks in each package's godoc and enforced with panics (internal callers) or
typed errors (boundary).

## Lightweight by design

- **No periodic tick.** A single `time.Timer` is set to fire at the next event
  boundary. Total wakeups per day ≈ 2 × events + midnight + a few refreshes.
- **5-minute timer cap.** macOS pauses the monotonic clock during sleep. Capping
  bounds wake-time skew; an extra check fires a refresh on detected wake.
- **No background polling.** Refresh on startup, midnight rollover, `r` keypress,
  and wake detection. Nothing else.
- **No `bubbles/*` sub-components.** Manual rendering with plain text plus a few
  unicode characters; no styling library overhead.
- **Read-only.** No event creation, no edit, no notifications, no themes, no
  config file. The single state file is the OAuth refresh token.

Idle target: <30MB RSS, ~0% CPU.

## Install

```
brew tap tombridger1030/tap
brew install --cask gcal
```

(Once a release is tagged. Until then, build from source — see below.)

## First-run setup

You'll need your own Google Cloud OAuth client (Google requires this for
desktop apps; takes about ten minutes once).

1. Go to https://console.cloud.google.com/projectcreate and create a project
   named `gcal` (or anything).
2. In that project, enable the Google Calendar API:
   APIs & Services → Library → search "Google Calendar API" → Enable.
3. Configure the OAuth consent screen (External, just for yourself):
   APIs & Services → OAuth consent screen → fill in the bare minimum, add
   your own Google account as a Test User, and set the scope to
   `https://www.googleapis.com/auth/calendar.readonly`.
4. Create OAuth credentials:
   APIs & Services → Credentials → Create Credentials → OAuth client ID →
   Application type **Desktop app** → Create. Download the JSON.
5. Hand the values to gcal. Pick whichever flow you prefer:

   **Easy path — let gcal prompt you (works with brew and any prebuilt
   binary).** Just run `gcal`. On first launch it prompts for your client
   ID, client secret, and an optional project ID, builds the
   `credentials.json` for you, and writes it to
   `~/Library/Application Support/gcal/credentials.json` (mode 0600).

   **Manual path — drop the downloaded JSON yourself.** Save Google's
   downloaded file directly to
   `~/Library/Application Support/gcal/credentials.json`.

   **Source build — embed at compile time.** Replace
   `internal/auth/credentials.json` in this repo with the downloaded file,
   then `go build -o gcal ./cmd/gcal`. The on-disk path still wins if you
   ever drop a file there later.

6. Continue. After credentials are in place, gcal opens your browser for
   consent. The refresh token is saved at
   `~/Library/Application Support/gcal/token.json` (mode 0600). Subsequent
   runs skip straight to the TUI.

## Usage

```
gcal              # launch the TUI
gcal --login      # re-run the OAuth consent flow (e.g. after revoke)
gcal --logout     # delete the local token
gcal --version    # print version and exit
```

Inside the TUI:

| Key      | Action                       |
| -------- | ---------------------------- |
| `q`      | quit                         |
| `Ctrl+C` | quit                         |
| `r`      | force-refresh today's events |

Pin a narrow terminal (about 32 columns, 30+ rows) to the side of your screen
and leave gcal running. It updates itself — block by block, day by day.

## Development

```
go test ./...
go vet ./...
go build ./cmd/gcal
```

Tests cover ~50 cases across pure helpers (schedule, render, timer math),
file storage, translation, and the Bubble Tea state machine. The OAuth
loopback flow and the actual Google API call are exercised by the binary at
runtime, not by unit tests.

## Releasing

Tagged pushes (`vX.Y.Z`) trigger
[`.github/workflows/release.yml`](.github/workflows/release.yml), which runs
[goreleaser](https://goreleaser.com) and:

- cross-compiles darwin-arm64 and darwin-amd64 binaries,
- uploads them as a GitHub Release,
- writes a Homebrew Cask formula to
  [`tombridger1030/homebrew-tap`](https://github.com/tombridger1030/homebrew-tap)
  (requires a `HOMEBREW_TAP_TOKEN` repo secret with `repo` scope on the tap).

To cut a release:

```
git tag v0.1.0
git push origin v0.1.0
```

Snapshot the release locally without publishing:

```
HOMEBREW_TAP_TOKEN=dummy goreleaser release --snapshot --clean --skip=publish
```
