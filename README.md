# wyndctl

A Go CLI for deploying Wynd Sentry IoT air-quality devices over USB serial. Handles device scanning, WiFi provisioning, property management, and device assignment against the Wynd backend database.

## Commands

| Command | Description |
|---|---|
| `scan` | Discover all Sentry devices connected via USB; optionally label bay positions |
| `deploy` | Provision WiFi credentials and assign devices to properties and rooms in parallel |
| `fw-update` | Write firmware images (main / WiFi / PMM) to connected devices over USB |
| `unprovision` | Clear WiFi and MQTT credentials from one or all devices |
| `create-account` | Create a user account (Stripe customer, Cognito user, platform DB row) |
| `create-property` | Create a lodging property for a user account |
| `list-property` | List lodging properties for a user account |
| `delete-property` | Delete a lodging property |
| `list-devices` | List devices assigned to a user account |
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

### Selecting an environment (dev / staging / prod)

`wyndctl` defaults to `dev`. Override per-command with `--env`, per-shell with
`WYND_ENV`, or persistently in `wyndctl.yaml`:

```bash
# One-off
wyndctl list-devices --env prod --account owner@example.com

# Whole shell session
export WYND_ENV=staging
wyndctl list-property --account owner@example.com
```

The chosen environment determines which `wynd-{env}-sentrydb` secret is fetched
from AWS Secrets Manager.

> **Running against `prod`?** Destructive commands (`delete-device`,
> `delete-property`, `unprovision`) have no confirmation prompt — that's
> intentional. See [docs/AWS_SETUP.md](docs/AWS_SETUP.md#safety-running-against-prod)
> for the safe-usage checklist.

### AWS credentials

The AWS SDK is bundled with `wyndctl` — nothing to install separately. You do
need credentials that can read Secrets Manager in `us-west-2`. The fastest path
is the AWS CLI:

```bash
brew install awscli         # macOS
# sudo apt install awscli   # Ubuntu / Debian

aws configure
# AWS Access Key ID:     AKIA...
# AWS Secret Access Key: ...
# Default region name:   us-west-2
# Default output format: json

# Sanity-check
aws sts get-caller-identity
```

Full walkthrough — named profiles, required IAM policy, and troubleshooting —
in [docs/AWS_SETUP.md](docs/AWS_SETUP.md).

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

### fw-update

Update firmware on all connected devices (any combination of images):

```bash
wyndctl fw-update --firmware sentry.bin
wyndctl fw-update --wifi-firmware bw16.bin
wyndctl fw-update --pmm-firmware pmm.bin
wyndctl fw-update --firmware sentry.bin --wifi-firmware bw16.bin --pmm-firmware pmm.bin
```

Force-update all devices regardless of a previous run's results:

```bash
wyndctl fw-update --firmware sentry.bin --all
```

Results are logged to `fw-update-result.csv`. Re-runs without `--all` only retry
devices that failed in the previous run.

The command waits after each reboot for the device to finish flashing
(~35 s main / ~140 s WiFi / ~100 s PMM) before returning.

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

### unprovision

Clear WiFi and MQTT credentials from all connected devices:

```bash
wyndctl unprovision
```

Target a single device by serial port:

```bash
wyndctl unprovision --port /dev/cu.usbmodem012345671
```

### create-account

Create a full user account (Stripe customer + Cognito user + platform DB row):

```bash
wyndctl create-account \
  --email owner@example.com \
  --name "Jane Smith" \
  --password "TempPass123!" \
  --enterprise-name "Acme Hotels" \
  --client-type hotel
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

### delete-property

Delete a lodging property:

```bash
wyndctl delete-property --account owner@example.com --lodging-id 42
```

### list-devices

List all devices assigned to a user account:

```bash
wyndctl list-devices --account owner@example.com
```

Scope to a specific property:

```bash
wyndctl list-devices --account owner@example.com --lodging-id 42
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

`wyndctl` automatically resolves credentials from AWS Secrets Manager using the secret name `wynd-{env}-sentrydb`. Ensure your AWS credentials are configured and have `secretsmanager:GetSecretValue` access in `us-west-2`. You can override with `WYND_DB_DSN` or by setting `db` values in `wyndctl.yaml`. See [docs/AWS_SETUP.md](docs/AWS_SETUP.md) for the full setup, including the minimal IAM policy and common error messages.

## Further reading

- [Architecture](docs/ARCHITECTURE.md) — transport stack, device lifecycle, command flows
- [AWS setup](docs/AWS_SETUP.md) — environments, AWS credentials, IAM policy, prod safety
- [Linux setup](docs/LINUX_SETUP.md) — dialout group, udev rules, build-on-target-OS
