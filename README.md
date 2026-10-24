# PM-WA

A WhatsApp task-list bot: groups chat task lists ("add-list", "add-task",
"done: 1") with reminders, plus a private per-admin web dashboard. One Go
process can serve several WhatsApp bot accounts, each fully isolated.

## Running

```sh
pm-wa              # run the bot server
pm-wa pair <name>  # pair a named account from the terminal
```

### Configuration (env vars)

| Variable         | Meaning                                                                                                   | Default |
|------------------|-----------------------------------------------------------------------------------------------------------|---------|
| `WA_ACCOUNTS`    | Comma-separated bot account names, e.g. `sales,service`. Unset means a single account named `default`.      | unset   |
| `WEB_BASE_URL`   | Public base URL of the web UI (e.g. `https://tasks.example.com`). Unset disables the web UI entirely.       | unset   |
| `WEB_LISTEN_ADDR`| Local listen address of the web server.                                                                   | `:8080` |

Account names must be URL-safe slugs: `[a-z0-9-]+` (lowercase letters,
digits, hyphens). Invalid or duplicate names fail fast at startup with a
clear error.

### Data layout

Every account owns an isolated data directory — its own session store and
its own task database. Task data never crosses accounts.

```
data/
  default/
    pm-wa.db        # task lists, tasks, web tokens/sessions
    pm-wa.db-wal
    pm-wa.db-shm
    wa-session.db   # WhatsApp linked-device session store
  sales/
    pm-wa.db
    wa-session.db
  ...
```

### Legacy auto-migration

On first boot, if the old root-level data files (`pm-wa.db`,
`wa-session.db`, and their `-wal`/`-shm` sidecars) exist and
`data/default/` does not yet exist, they are moved into `data/default/`
with one-time rename semantics before anything connects. No manual step is
needed when upgrading an existing single-account install.

Notes:

- The legacy backup database at the repo root (`pm-wa-server.db`) is never
  touched or moved.
- If `data/default/` already exists, any legacy root files are left
  untouched and a warning is logged.

### Pairing (`pm-wa pair <name>`)

1. Add the account name to `WA_ACCOUNTS` (e.g. `WA_ACCOUNTS=sales`).
2. Run `pm-wa pair sales`. The command creates `data/sales/`, prints a QR
   code to stdout, and blocks.
3. On the phone that owns the target number: **WhatsApp > Settings >
   Linked devices > Link a device**, then scan.
4. When pairing completes, the command exits. The paired account connects
   normally on the next `pm-wa` start.

An unconfigured name fails fast, listing the accounts that are configured.

**Re-pairing (recovery):** if an account was logged out from the phone, its
stored session is dead. Run the same subcommand; it detects the stored
session and asks `Discard and re-pair? [y/N]` before dropping it. Onboarding
and recovery are the same flow.

The default account (unset `WA_ACCOUNTS`) also pairs with the same
subcommand, but booting it without a session still shows the boot-time QR
loop, as before.

## Multiple accounts: operational rules

- **One bot per group.** A group must be served by exactly one account.
  Adding two accounts to the same group is unsupported and will cause
  duplicate replies and conflicting task data — there is no arbitration or
  enforcement in the code, so keep ownership exclusive when inviting bots
  to groups. A group's task lists belong to the account that serves it.
- Failure isolation: an account that is unpaired or cannot connect is
  logged and skipped at startup; healthy accounts keep serving, and each
  account's reminder scheduler restarts automatically when that account
  reconnects after a transient drop.
- Full feature parity: every account gets task lists, replies, cron
  pulses, and its own web UI.

## Web UI

With multiple accounts, one listener serves each account under its own
prefix:

- `/a/<name>/` — that account's dashboard, backed by its own database.
- `/a/<name>` (no slash) redirects to the account's lists page.
- Auth is per account: login links (`tasklist-web`) and sessions belong to
  the account that sent them; each account's browser cookie is named
  `pmwa_sess_<name>` and scoped to its own prefix.
- The navbar has an account switcher to jump between dashboards.
- With `WA_ACCOUNTS` unset, the UI is served at the bare root with the
  legacy `pmwa_sess` cookie — identical to the pre-multi-account behavior.

### Connection status badge

When multiple accounts are configured, the navbar shows a badge per
account:

| Badge             | Meaning                                                                                      |
|-------------------|----------------------------------------------------------------------------------------------|
| `connected`       | The account is linked and connected to WhatsApp.                                             |
| `pairing-required`| The account has no stored session — run `pm-wa pair <name>`.                                 |
| `error`           | The account is paired but not connected right now (connect failure or transient disconnect).  |
| `unknown`         | Status is unavailable; rendered degraded instead of failing the page.                         |

The badge reflects the state at page render; reload to see a reconnect.
Connection failures are also logged with the account name and error.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

Live verification (pairing with a real number, two real accounts serving
groups, reconnect behavior) requires real WhatsApp sessions and is done
manually — automated tests cover the pure resolution logic and the HTTP
handler seam.
