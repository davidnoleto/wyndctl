# Environments and AWS setup

`wyndctl` talks to one of three Wynd backend environments — **dev**, **staging**, or **prod**.
Database credentials for the chosen environment are pulled at runtime from AWS
Secrets Manager in `us-west-2`, so you need working AWS credentials before any
command that hits the database will work.

## Selecting an environment

The environment is `dev` by default. You can override it from any of the
standard configuration sources (highest priority first):

| Source | Example |
|---|---|
| CLI flag | `wyndctl list-devices --env prod --account owner@example.com` |
| `WYND_ENV` env var | `WYND_ENV=staging wyndctl list-property --account ...` |
| `wyndctl.yaml` | `env: prod` |
| Built-in default | `dev` |

The chosen environment drives:

- which `wynd-{env}-sentrydb` secret is read from AWS Secrets Manager
- which `configs/{env}.env` file is auto-loaded from `~/code/wynd-sentry/backend/`
  (if present — only relevant if you have the backend checked out locally)
- which Stripe and Cognito secrets are resolved for `create-account`

### Quick switching

```bash
# One-off prod command
wyndctl list-devices --env prod --account owner@example.com

# Whole shell session against staging
export WYND_ENV=staging
wyndctl list-property --account owner@example.com
```

### Overriding the database directly

If you need to point at a database that isn't in Secrets Manager (e.g. a local
Postgres), set `WYND_DB_DSN` and AWS resolution is skipped entirely:

```bash
export WYND_DB_DSN="postgres://user:pass@localhost:5432/sentry?sslmode=disable"
wyndctl list-devices --account owner@example.com
```

You can also set individual `db.*` keys in `wyndctl.yaml`; any value other than
an empty string or an `aws:secret:...` reference is used as-is.

## AWS credentials

`wyndctl` uses the AWS SDK for Go v2's default credential chain. In practice
that means it picks up credentials from, in order:

1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`)
2. `~/.aws/credentials` (selected by `AWS_PROFILE`, defaults to `[default]`)
3. IAM role on the host (EC2 / ECS / Lambda)

The SDK itself is bundled with `wyndctl` — there is nothing to install
separately. You only need to install the AWS CLI if you want a convenient way
to write your credentials to `~/.aws/credentials`.

### Setting up static keys with `aws configure`

The simplest setup:

```bash
# macOS
brew install awscli

# Ubuntu / Debian
sudo apt install awscli

# Configure default profile
aws configure
# AWS Access Key ID [None]: AKIA...
# AWS Secret Access Key [None]: ...
# Default region name [None]: us-west-2
# Default output format [None]: json
```

Verify it works:

```bash
aws sts get-caller-identity
aws secretsmanager get-secret-value \
  --region us-west-2 \
  --secret-id wynd-dev-sentrydb \
  --query SecretString --output text | head -c 60
```

If the second command returns a JSON blob with `host`/`port`/`username`/`password`
keys, `wyndctl` will be able to resolve `--env dev` against that database.

### Using a named profile

If you have multiple AWS accounts, put credentials under a named profile and
pick it with `AWS_PROFILE`:

```ini
# ~/.aws/credentials
[wynd]
aws_access_key_id     = AKIA...
aws_secret_access_key = ...
```

```bash
export AWS_PROFILE=wynd
wyndctl list-devices --env prod --account owner@example.com
```

### Required IAM permissions

The IAM user or role you use needs to be able to read the Sentry secrets in
`us-west-2`. A minimal policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "secretsmanager:GetSecretValue",
      "Resource": [
        "arn:aws:secretsmanager:us-west-2:*:secret:wynd-dev-sentrydb-*",
        "arn:aws:secretsmanager:us-west-2:*:secret:wynd-staging-sentrydb-*",
        "arn:aws:secretsmanager:us-west-2:*:secret:wynd-prod-sentrydb-*",
        "arn:aws:secretsmanager:us-west-2:*:secret:wynd-*-stripe-api-key-*"
      ]
    }
  ]
}
```

If you'll be running `create-account`, you also need Cognito user-pool admin
permissions for the environment's user pool. Ask the platform team for the
exact pool ID and an attached policy.

### Troubleshooting

| Symptom | Likely cause |
|---|---|
| `loading AWS config: ... no EC2 IMDS role found, operation error ec2imds` | No credentials anywhere on the chain. Run `aws configure` or set `AWS_PROFILE`. |
| `fetching secret "wynd-prod-sentrydb": ... AccessDeniedException` | Credentials work but lack `secretsmanager:GetSecretValue` on that secret. Update the IAM policy. |
| `fetching secret ...: ... ResourceNotFoundException` | Wrong region or wrong env name. Secrets live in `us-west-2`; check `--env`. |
| Wrong account / wrong environment | `AWS_PROFILE` is pointing at a different account. Verify with `aws sts get-caller-identity`. |

## Safety: running against `prod`

Several `wyndctl` commands are destructive and have **no confirmation prompt**
(intentionally — see `CLAUDE.md`). Against prod, this means a wrong flag is
production data loss.

**Always check what env you're in before running destructive commands:**

```bash
echo "WYND_ENV=${WYND_ENV:-unset}"
grep '^env:' wyndctl.yaml 2>/dev/null
aws sts get-caller-identity  # confirms which AWS account / role
```

Commands to be extra careful with on `--env prod`:

- `delete-device` — removes device-to-room assignments; no prompt.
- `delete-property` — deletes a lodging property row.
- `unprovision` — wipes WiFi/MQTT creds from a physical device (USB only, so
  blast radius is whatever's plugged in, but still — make sure the device on
  your desk is the one you mean).
- `deploy` / `fw-update` — physical device side effects; check
  `deployment-data.csv` / `--firmware` paths twice.
- `create-account` — creates real billable Stripe customers in prod.

Recommended workflow:

1. Do a dry inspection first: `wyndctl list-property --env prod --account <email>`
   and `wyndctl list-devices --env prod --account <email>` to confirm the IDs
   you're about to act on.
2. Scope deletes tightly with `--lodging-id` rather than account-wide.
3. If you're rehearsing a flow, run it against `--env dev` first. Dev shares
   the schema with prod, so the same flag set works against both.
4. Never leave `WYND_ENV=prod` exported in a long-lived shell. Set it inline
   for the single command instead: `WYND_ENV=prod wyndctl ...`.
