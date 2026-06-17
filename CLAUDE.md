# CLAUDE.md

A focused Go CLI for deploying Wynd Sentry IoT air-quality devices over USB serial
and managing the platform accounts/properties they're assigned to. Commands:
`scan`, `deploy`, `fw-update`, `unprovision`, `create-account`, `create-property`,
`list-property`, `delete-property`, `list-devices`, `delete-device`. A minimal
subset of the full `wynd-deploy-cli`.

## Commands

- `make build` / `make install` — **always use these, not plain `go build`/`go install`.**
  The Makefile injects build metadata (`version`, `buildTime`, `gitSHA`, `gitDirty`,
  `maxAgeDays=90`) via `-ldflags -X`. Without it, `main.checkBinaryAge()` silently
  disables itself and the 90-day kill-date safeguard does nothing.
- `make test` — runs with `-race`. Use `make test-short` to skip slow tests.
- `make build-all` — cross-compile linux/darwin × amd64/arm64.
- See `Makefile` for anything else.

## Architecture

Read on demand, not every session:

- Overview, layer map, device lifecycle: `docs/ARCHITECTURE.md`
- Transport stack (COBS → packet → RPC → encoding): `docs/ARCHITECTURE.md` § Transport
- Command flows (scan, deploy, fw-update, create-account, delete-device, etc.): `docs/ARCHITECTURE.md` § Command flows
- Linux discovery troubleshooting (dialout, udev, build-on-target-OS): `docs/LINUX_SETUP.md`

## Conventions

- Cobra commands: use `cmd.OutOrStdout()` and `cmd.ErrOrStderr()`.
  Never `fmt.Println`/`fmt.Printf` inside a command.
- Errors: wrap with `fmt.Errorf("context: %w", err)`. Don't replace errors with
  `errors.New` in non-leaf code.
- No new interfaces unless there are at least two implementations.
- No new dependencies without justification — prefer standard library.
- Keep `cmd/*` thin: orchestration only, real logic lives in `internal/`.

## Configuration priority (highest → lowest)

1. CLI flags
2. `WYND_*` env vars (e.g. `WYND_DB_DSN`)
3. `wyndctl.yaml`
4. `PG_*` / `IOT_*` env vars (Python CLI compat)
5. `configs/{env}.env` auto-discovered from `~/code/wynd-sentry/backend/`
6. Built-in defaults

If `db.host` is empty or looks like an AWS secret reference, config auto-resolves
from the `wynd-{env}-sentrydb` secret in AWS Secrets Manager (us-west-2).

## Security — hard rules

- **Never log the `--password` argument to `create-account`.** The `SECURITY:`
  comment in `runCreateAccount` (cmd/account.go) is load-bearing.
- **Never detach IoT certs or policies.** Sentry devices are factory pre-provisioned
  with X.509 certs that are permanent hardware identity. `EnsureThing` only sets
  `assigned=true`; `UnassignThing` only sets `assigned=false`. Anything that touches
  certs or policies breaks the device irrecoverably.
- The *absence* of a confirmation prompt on `delete-device` is also intentional.
  Don't add one.

## Don't

- Don't add `.proto` files or pull in `google.golang.org/protobuf`. The hand-rolled
  encoding in `internal/device/proto.go` is intentional; field IDs are wire-compatible
  with deployed firmware. A "cleanup" refactor here breaks production devices.
- Don't change the 9600 baud rate, USB VID `0x2fe3`, or PID `0x0100` — hardware-fixed.
- Don't add abstractions in anticipation of future needs. This codebase is
  deliberately a minimal subset of the full deploy CLI; resist the urge to
  "professionalize" it.
- Don't add `internal/` sub-splits without a real reason. Current layout is the layout.
- Don't commit `deployment-result.csv`, `fw-update-result.csv`, or `location-map.csv` — runtime artifacts.
- Don't commit `*.bin` firmware files.
- Don't run plain `go install`/`go build` (see Commands above).

## Linux gotcha

When a user reports `wyndctl scan` finds no devices on Linux, **first check that the
binary was built on the Linux host.** A macOS-built binary copied to Linux fails
USB discovery silently, producing the same "no devices found" message as a
permissions issue. Send them to `docs/LINUX_SETUP.md` for the full flow
(dialout group, udev rules, build-on-target-OS).
