# Architecture

Detailed reference for the `wyndctl` codebase. Loaded on demand when working
on a specific area — most sessions don't need this file. See `CLAUDE.md` for
conventions, security rules, and hard "don'ts".

## Layer overview

```
cmd/           Cobra commands — thin orchestration only
internal/
  config/      Viper-based config loading + AWS Secrets Manager resolution
  transport/   USB serial I/O: COBS framing → packet layer → RPC layer
  device/      High-level device commands (Commander) + protobuf-lite encoding
  deployment/  Deployment orchestration (Service), CSV I/O
  iot/         AWS IoT device lifecycle (EnsureThing, UnassignThing)
  database/    GORM repository — methods deploy and delete-device need
  models/      Shared domain types
  logger/      slog wrapper
```

Real logic lives under `internal/`. `cmd/*` only wires flags to calls into
`internal/`.

## Device command flow

```
cmd/*  →  device.Commander  →  transport.RPC  →  transport.SerialChannel  →  USB
```

`Commander` (`internal/device/commander.go`) is the single entry point for all
device interactions. If you need a new device operation, add a method there —
don't reach into the transport layer from `cmd/`.

## Transport

Sentry devices speak a custom binary RPC over 9600-baud USB serial. The stack,
bottom to top:

1. **COBS** (`internal/transport/cobs.go`) — packet framing. `0x00` is the
   delimiter byte; payloads are COBS-encoded to remove zero bytes.
2. **Packet** (`internal/transport/packet.go`) — typed packets
   (ServerRequest, ClientResponse, …) with a CRC16 integrity check.
3. **RPC** (`internal/transport/rpc.go`) — multiplexed unary calls identified by
   `(packageID, serviceID, methodID, invocationID)`. Multiple in-flight calls
   on a single channel are tracked by invocationID.
4. **Encoding** (`internal/device/encoding.go`, `internal/device/proto.go`) —
   hand-rolled protobuf-compatible wire encoding. There are no `.proto` files;
   field IDs are hardcoded constants in `proto.go`. This is intentional and
   wire-compatible with deployed firmware — see `CLAUDE.md` § Don't.

USB device discovery uses VID `0x2fe3` / PID `0x0100`. These are hardware-fixed.

## Device lifecycle

Sentry devices are **factory pre-provisioned**: each device arrives with an
X.509 certificate already registered in AWS IoT. The cert and its policy are
permanent hardware identity and must never be deleted or detached.

- `EnsureThing` — creates the Thing if missing, sets `assigned=true` attribute.
  Never touches certs or policies.
- `UnassignThing` — sets `assigned=false`. Idempotent on `ResourceNotFoundException`.
  Never touches certs or policies.

Anything that detaches certs or policies breaks the device irrecoverably and
cannot be fixed in the field.

## Command flows

### `scan`

1. `commander.Scan()` calls `transport.FindSerialPorts()` — discovers USB
   devices by VID `0x2fe3` / PID `0x0100`.
2. Opens each port via `ActivateAll()`.
3. Calls `GetDeviceInfo` (RPC method 1) on each channel — prints firmware versions.
4. `--label` mode: lights each LED, waits for operator to type a bay number,
   writes `location-map.csv`.
5. `--output json` (or `-o json`) emits a JSON array; falls back to
   `--log-format` if `--output` is not set.

### `deploy`

1. Loads `deployment-data.csv` (bay → WiFi/MQTT/room config) and optionally
   `location-map.csv` (USB port → bay).
2. Scans USB for devices (same as `scan`).
3. For each device in parallel (goroutine pool, `--workers` controls concurrency):
   - `commander.Unprovision()` — clears old creds.
   - `commander.SetAdvertising(false)` — disables BLE.
   - `commander.SetProvision(ssid, psk, ...)` — sends WiFi+MQTT creds, polls
     until `ProvisionMQTTPublish`.
   - `iot.EnsureThing()` — creates/marks AWS IoT Thing as assigned.
     Optional, skipped if AWS unavailable.
   - `repo.AssignDeviceToZone()` — writes device→room to Postgres.
     Optional, skipped if DB unavailable.
   - LED feedback + appends row to `deployment-result.csv`.

The `--env prod` confirmation gate (`requireProdConfirmation()`) fires early
in `RunE`. `--dry-run` bypasses it.

### `delete-device`

```
wyndctl delete-device --account <email> [--lodging-id N] [--device-id THING]
```

1. `repo.DeleteDevices(ownerID, lodgingID?, deviceID?, deleteRooms=true)` —
   joins `device → zone → lodging` filtered by `lodging.owner_id`. For each
   match, nulls `device.zone_id` (keeps the device row so it can be redeployed)
   and deletes the zone row. Returns the affected Thing names. Room deletion
   is always on; there is no flag.
2. `iot.UnassignThing()` — for each Thing, sets attribute `assigned=false`.
   Idempotent on `ResourceNotFoundException`. **Never** detaches certs or
   policies. DB clears even if some IoT updates fail; the command exits
   non-zero with a count.

No prod gate, no interactive confirm — by design.

## Files in the working directory

| File | Direction | Purpose |
|---|---|---|
| `deployment-data.csv` | Input | bay → WiFi SSID/PSK, account, lodging_id, room. Contains plaintext PSKs — treat as secret. |
| `location-map.csv` | Input (optional) | USB port location → bay number. Created by `scan --label`. |
| `deployment-result.csv` | Output | Per-device result log. Don't commit. |

## AWS

All AWS calls use the default credential chain (SSO / env vars / `~/.aws`).
Region is hardcoded to `us-west-2`.

- **Secrets Manager** — DB credentials at `wynd-{env}-sentrydb`.
- **IoT** — `EnsureThing` / `UnassignThing` only flip the `assigned` attribute.

## Database

Postgres `sentry` DB, accessed via GORM. Source of truth for the schema is the
GORM models in `internal/models/`. Tables involved:

- `user` — email, full_name
- `lodging` — property, owned by user
- `zone` — room, belongs to lodging
- `device` — `device_id` = AWS Thing name, nullable `zone_id`

The `delete-device` join walks `device → zone → lodging` filtered by
`lodging.owner_id`. Setting `device.zone_id = NULL` (rather than deleting the
device row) is deliberate: it allows redeployment of the same physical hardware
without needing to re-register the IoT Thing.
