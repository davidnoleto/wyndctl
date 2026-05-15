# wyndctl

A Go CLI for deploying Wynd Sentry IoT air-quality devices over USB serial. Handles device scanning, WiFi provisioning, property management, and device assignment against the Wynd backend database.

## Commands

| Command | Description |
|---|---|
| `scan` | Discover all Sentry devices connected via USB; optionally label bay positions |
| `deploy` | Provision WiFi credentials and assign devices to properties and rooms in parallel |
| `create-property` | Create a lodging property for a user account |
| `list-property` | List lodging properties for a user account |
| `delete-device` | Remove device-to-room assignments from the database |

## Requirements

- Go 1.24+
- AWS credentials with access to Secrets Manager (for automatic DB credential resolution)
- USB access to connected Sentry devices

## Installation

### macOS

Both Apple Silicon (arm64) and Intel (amd64) are supported.

```bash
git clone https://github.com/davidnoleto/wyndctl.git
cd wyndctl
make install
```

`wyndctl` will be installed to your `$GOPATH/bin`. Make sure that is on your `$PATH`.

### Linux

> **Important:** the binary must be built on the Linux host. A macOS-built binary copied to Linux will fail USB device discovery silently with no error.

```bash
git clone https://github.com/davidnoleto/wyndctl.git
cd wyndctl
make install
```

Additional setup is required on Linux for USB serial access (dialout group membership, udev rules). See [docs/LINUX_SETUP.md](docs/LINUX_SETUP.md) for the full walkthrough.

### Windows

Not yet tested. May work but is not officially supported.

## Configuration

Configuration is resolved in this priority order (highest to lowest):

| Source | Example |
|---|---|
| CLI flags | `--env prod` |
| `WYND_*` env vars | `WYND_DB_DSN=postgres://...` |
| `wyndctl.yaml` | see below |
| `PG_*` / `IOT_*` env vars | Python CLI compatibility |
| Auto-discovered backend `.env` | `~/code/wynd-sentry/backend/configs/{env}.env` |
| Built-in defaults | `env=dev` |

If `db.host` is empty or references an AWS secret, credentials are automatically resolved from the `wynd-{env}-sentrydb` secret in AWS Secrets Manager (us-west-2).

Minimal `wyndctl.yaml`:

```yaml
env: dev   # dev, staging, or prod

log:
  level: info
  format: text  # text or json
```

Place this file in the directory where you run `wyndctl`, or pass `--config /path/to/wyndctl.yaml`.

## Usage

### scan

Discover all connected Sentry devices and light their LEDs:

```bash
wyndctl scan
```

Interactively label each device's bay position and write a `location-map.csv`:

```bash
wyndctl scan --label
```

### deploy

Provision all connected devices in parallel using `deployment-data.csv`:

```bash
wyndctl deploy --all
```

Re-run only devices that failed in the previous attempt:

```bash
wyndctl deploy
```

Deploy one device at a time with operator confirmation between each:

```bash
wyndctl deploy --iterative
```

### create-property

Create a lodging property for a user account:

```bash
wyndctl create-property \
  --account owner@example.com \
  --name "Sunset Hotel" \
  --address "123 Ocean Ave" \
  --city "Santa Monica" \
  --state "CA" \
  --country "US" \
  --zip "90401"
```

### list-property

List all properties for a user account:

```bash
wyndctl list-property --account owner@example.com
```

### delete-device

Remove all device assignments for a user:

```bash
wyndctl delete-device --account owner@example.com
```

Scope to a specific property:

```bash
wyndctl delete-device --account owner@example.com --lodging-id 42
```

Also delete the associated room records:

```bash
wyndctl delete-device --account owner@example.com --lodging-id 42 --delete-room
```

## Building from source

```bash
# Build binary to bin/wyndctl
make build

# Install to $GOPATH/bin
make install

# Cross-compile for all platforms
make build-all

# Run tests with race detector
make test

# Run tests, skip slow ones
make test-short
```

> Always use `make build` / `make install` rather than plain `go build` / `go install`. The Makefile injects build metadata used by the binary age check.

## Troubleshooting

### `scan` finds no devices on Linux

First check that the binary was built on the Linux host — a macOS-compiled binary copied to Linux produces the same "no devices found" output as a permissions error. See [docs/LINUX_SETUP.md](docs/LINUX_SETUP.md) for the full setup (dialout group, udev rules, build-on-target-OS).

### Database connection errors

`wyndctl` automatically resolves credentials from AWS Secrets Manager using the secret name `wynd-{env}-sentrydb`. Ensure your AWS credentials are configured and have `secretsmanager:GetSecretValue` access in `us-west-2`. You can override with `WYND_DB_DSN` or by setting `db` values in `wyndctl.yaml`.

## Further reading

- [Architecture](docs/ARCHITECTURE.md) — transport stack, device lifecycle, command flows
- [Linux setup](docs/LINUX_SETUP.md) — dialout group, udev rules, build-on-target-OS
