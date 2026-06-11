# gcal

A tiny terminal calendar that pins to the side of your Mac screen and shows
today's Google Calendar as a column of time-blocks. The "current" indicator
auto-advances at every event boundary; the day rolls over at local midnight.
At each top-of-hour where the prior hour overlaps a calendar block whose
title contains `work`, gcal asks for a 1-5 focus rating and appends it to a
private local journal.

Events are merged from every calendar you have marked **visible** in the
Google Calendar UI (work + personal + shared, etc.), not just `primary`.

```
┌──────────────────────────────┐
│ Tue · May 13                 │
│ ──────────────────────────── │
│ all-day: OOO – Jamie         │
│                              │
│   09:00–09:30  standup       │
│ ▌ 10:30–11:30  Design  ◀ NOW │
│ ▌              Zoom          │
│   12:00–13:00  lunch w/ Sam  │
│                Tartine       │
│   14:00–15:00  1:1 w/ Pat    │
└──────────────────────────────┘
```

Each event is one line ("HH:MM–HH:MM title"); location, when present,
drops to a second indented line. A busy 14-event day fits in ~17 rows
so the column stays usable even when pinned to a short pane.

## Design

Seven small Go packages, each a "deep module" (Ousterhout): narrow public
interface, substantial hidden implementation.

| Package             | Public surface                                        | Hides                                                                                                                            |
| ------------------- | ----------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `internal/schedule` | `BuildState`, `NextTransition`                        | sort, clamp, classify, midnight math, DST                                                                                        |
| `internal/calendar` | `New`, `FetchDay`                                     | Google SDK, RFC3339, recurring expansion, declined-invite filter, multi-calendar merge across `CalendarList`, pagination drain   |
| `internal/auth`     | `TokenStore`, `RunFirstTimeFlow`, `EnsureCredentials` | OAuth loopback server, PKCE, atomic 0600 writes, interactive first-run credentials prompt, disk-then-embedded credentials lookup |
| `internal/focus`    | `Entry`, `NextPromptAt`, `DefaultJournal`             | work-block prompt math, append-only JSONL journal, 0600 writes                                                                   |
| `internal/notify`   | `Notifier`, `Osascript`                               | macOS notification scripting and AppleScript escaping                                                                            |
| `internal/ui`       | `Run`                                                 | Bubble Tea model, transition/focus timers, render layout                                                                         |
| `cmd/gcal`          | `main`                                                | flag parsing, wiring                                                                                                             |

Function preconditions and postconditions are documented as Design-by-Contract
blocks in each package's godoc and enforced with panics (internal callers) or
typed errors (boundary).

## Lightweight by design

- **No periodic tick.** Capped `time.Timer`s are set for the next event
  boundary and the next focus prompt. Wakeups are tied to real state changes,
  not a constant polling loop.
- **5-minute timer cap.** macOS pauses the monotonic clock during sleep. Capping
  bounds wake-time skew; an extra check fires a refresh on detected wake.
- **No background polling.** Refresh on startup, midnight rollover, `r` keypress,
  wake detection, and capped focus-prompt timers. Nothing else.
- **No `bubbles/*` sub-components.** Manual rendering with plain text plus a few
  unicode characters; no styling library overhead.
- **Calendar read-only.** No event creation, no event edits, no themes, no
  user-settings file. The on-disk state is the OAuth client credentials,
  refresh token, and `focus.jsonl` focus journal, all under
  `~/Library/Application Support/gcal/` with private file permissions.

Idle target: <30MB RSS, ~0% CPU.

## Install

```
brew tap tombridger1030/tap
brew install --cask gcal
```

Latest released version: see the
[Releases page](https://github.com/tombridger1030/gcal/releases). The cask
strips the macOS quarantine attribute on install so the unsigned binary
runs without a Gatekeeper prompt.

Prefer to build from source? See the **Development** section below.

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
gcal --logout     # delete the local token (no-op if not logged in)
gcal --no-focus   # launch without focus check-ins for this session
gcal --focus-log  # print the local focus journal
gcal --version    # print version and exit
```

Inside the TUI:

| Key      | Action                       |
| -------- | ---------------------------- |
| `q`      | quit                         |
| `Ctrl+C` | quit                         |
| `r`      | force-refresh today's events |
| `1`-`5`  | answer the focus prompt      |
| `s`      | skip the focus prompt        |

Focus ratings are appended to
`~/Library/Application Support/gcal/focus.jsonl` as JSON lines. A focus prompt
fires only when the completed prior hour overlaps a timed calendar block whose
title contains `work` (case-insensitive).

Pin a narrow terminal (about 32 columns, 30+ rows) to the side of your screen
and leave gcal running. It updates itself — block by block, day by day.

## Development

```
go test ./...
go vet ./...
go build ./cmd/gcal
```

For a fast manual focus-check loop, set `GCAL_FOCUS_INTERVAL=60s` before
running `go run ./cmd/gcal`. The interval override still requires the
completed hour to overlap a `work`-titled calendar block.

Tests cover pure helpers (schedule, focus, render, timer math),
file storage, translation, the Bubble Tea state machine, and the
interactive credentials prompt (driven via `strings.Reader` against a
pinned `t.TempDir`). The OAuth loopback flow and the actual Google API
call are exercised by the binary at runtime, not by unit tests.

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
git tag vX.Y.Z
git push origin vX.Y.Z
```

Snapshot the release locally without publishing:

```
HOMEBREW_TAP_TOKEN=dummy goreleaser release --snapshot --clean --skip=publish
```
