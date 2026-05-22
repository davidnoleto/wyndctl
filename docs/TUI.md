# Terminal UI (experimental)

wyndctl ships a Bubble Tea-based terminal UI for a single command today:
[`fw-update-tui`](../README.md#fw-update-tui). This document explains the
intent and the conventions that future TUI work should follow.

## Why TUIs are opt-in

The default commands stream structured log lines to stderr and write durable
results to CSV. That keeps the CLI scriptable — anything piped into `jq`,
parsed in CI, or scraped from a deploy log keeps working.

A TUI is the opposite contract: it takes over the terminal, redraws on every
tick, and is meant to be watched by a human. The two contracts can't share
the same exit path without breaking scripts, so the project's convention is:

- **Each TUI is a separate command** (e.g., `fw-update-tui` mirrors
  `fw-update`). The non-TUI command remains the canonical, scriptable entry
  point.
- **The TUI command must redirect slog** to a file or buffer while it's
  running so the structured logs don't fight the UI. `fw-update-tui` writes
  to `fw-update-tui.log` in the working directory.
- **Result files stay the same.** `fw-update` and `fw-update-tui` both write
  `fw-update-result.csv` with the same schema. The TUI is a presentation
  layer over identical worker logic.

## Where the TUI lives

```
internal/tui/
  events.go      // shared progress-event types (FWStage, FWProgressEvent, …)
  fwupdate.go    // the fw-update Bubble Tea model + View
cmd/
  fwupdate.go         // shared worker logic; emits events via a callback
  fwupdate_tui.go     // the fw-update-tui Cobra command
```

The worker in `cmd/fwupdate.go` takes an optional `tui.FWProgressFunc`
callback. The non-TUI command passes `nil` and behaves exactly as before.
The TUI command wires the callback to push events into the running
`tea.Program`. This keeps the TUI off the hot path for scripted users and
makes the whole subsystem cleanly deletable.

## Conventions for future TUIs

If you add a TUI for another command, follow this shape:

1. **New Cobra command, not a flag.** `<thing>-tui` next to `<thing>`. Don't
   gate the TUI behind a flag on the existing command — it changes the
   default contract for everyone who already automates it.
2. **Add a `<Thing>Stage` enum and `<Thing>ProgressEvent` to
   `internal/tui/events.go`.** Events should carry enough context to render
   without reaching back into command state.
3. **Pass progress as an optional callback** to the worker. `nil` must be a
   no-op so the non-TUI command compiles and runs without the TUI package
   touching its code path.
4. **Redirect slog** to a file in the TUI command's `RunE` before calling
   `tea.NewProgram(...).Run()`. Restore on exit.
5. **Keep the durable artifact (CSV, JSON, etc.) unchanged.** The TUI is for
   the human watching the run; the result file is for everything else.

## Why this is parked as "experimental"

`fw-update-tui` exists as a proof of concept. It works, it's been verified
on real hardware, but it's not yet the recommended way to run firmware
updates from scripts or shared documentation. Keep the TUI explicitly opt-in
until there's a clear reason to make it the default (and even then, only via
a separate command name, never by changing `fw-update`).

Candidates for the next TUI, if we ever extend this:

- `deploy` — the multi-step provisioning flow (scan → pick → enter creds →
  provision → verify) maps cleanly onto a wizard. The CSV-batch path
  (`deploy --all`) should stay log-based.
- `scan --watch` — a live list of connected devices that updates as boards
  are plugged or pulled.

Neither is on the roadmap. They're listed here so anyone considering "should
I add a TUI for X?" has prior art to look at first.

## Dependencies

The TUI package pulls in:

- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/bubbles`
- `github.com/charmbracelet/lipgloss`

These transitively add ~25 indirect dependencies and ~2-3 MB to the binary.
That cost is acceptable for the current single-command use case. If we ever
decide the TUI direction isn't worth it, deleting `cmd/fwupdate_tui.go` and
`internal/tui/` and running `go mod tidy` removes the entire surface.
