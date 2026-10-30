# AGENTS.md

Instructions for AI agents working in this repository (PM-WA — a WhatsApp task-list bot with a Go web UI, SQLite storage, and the whatsmeow client).

## Performance first: always produce optimized code

This app talks to live WhatsApp servers over the network. Latency bugs lock up the UI just like code bugs do. Every change must follow these rules:

### Never put network calls inside loops

- Before writing a loop, check whether any call inside it performs RPC / network / I/O. If it does, find a **batch API** that does it in one call instead.
- Precedent: `bot.MemberGroupJIDs` + `GetMemberGroups` (`bot/group.go`) replaced N per-group `GetGroupInfo` calls with a single `GetJoinedGroups` call. Follow that shape for anything similar.
- Prefer whitelisting data fetched once (e.g., joined groups with participants) over re-fetching per item, then compute locally with a pure function.

### Prefer local, synced data over live RPCs

- Read-heavy UI data that rarely changes (group names, membership) belongs in SQLite (`wa_groups`, `task_lists`) and is kept fresh by real-time events (see `main.go` upserts). Render pages from the DB; use RPC only for correctness-critical checks (e.g., validating a submitted form value).
- Filter/intersect in memory against already-loaded data instead of round-tripping.

### Always bound outbound calls with a context deadline

- Any handler that triggers an RPC must wrap the request context with `context.WithTimeout` and fail fast (a few seconds max, well under the server's `WriteTimeout` = 15s in `web/server.go`).
- On RPC failure: log with `slog.Error`, render the page degraded (empty options / friendly error), never hang.

### Keep behavior parity when optimizing

- When replacing a slow path, preserve its exact semantics (including quirks like the LID-only optimistic-membership rule) unless the plan explicitly says otherwise. Port any `slog.Warn` that documented behavior.
- State in the commit/PR that semantics are unchanged.

### Database hygiene

- SQLite: no N+1 queries; fetch siblings in one query and join in memory.
- Use transactions for multi-statement writes (see `db/web.go` `ReplaceTaskAssignees`).

## General conventions

- Go 1.21+ style: `log/slog` for logging, errors returned as `error`, no panics in library code.
- Keep handlers thin: `web/handlers.go` orchestrates, `bot` wraps whatsmeow, `db` owns SQL, `format` owns message rendering.
- Don't add fields or helpers that duplicate something that already exists — search first (`db/`, `bot/`, `cmd/`) and reuse.
- Store times as Unix seconds (int64); user-facing parsing/formatting uses the `gmt7` zone (`time.FixedZone("GMT+7", ...)`).

## Testing

- Every new exported function — especially pure helpers — ships with table-driven unit tests in an external `X_test` package next to the code (see `bot/group_test.go`).
- Cover: success, every error path, boundaries (nil/empty/zero/unparseable inputs).
- No generated mock infrastructure exists for `*bot.Client` / `*db.Database` (concrete structs); design logic as pure functions over plain inputs so it is testable without mocks.

## Verification before finishing

Run and report actual results (never claim a pass you didn't observe):

```sh
go build ./...
go vet ./...
go test ./...
```

There is no Makefile or separate lint target; `go vet` is the lint gate. Manual checks requiring a live WhatsApp session must be flagged as manual, not implied to pass.

## Deployment

Production runs on host `emangtop@103.30.194.149`, in the tmux session `pm-wa`, from the checkout at `/home/emangtop/PM-WA`. nginx (`/etc/nginx/conf.d/reminder-wa.conf`) proxies `reminder-wa.emang.top` to `127.0.0.1:8888`.

The two env vars below are defined **nowhere** on the host (no `.bashrc`, no `.env`, no systemd) — they must be exported in the pane at start. A tmux server inherits env from whatever started it, so if the server is ever recreated (e.g. after a crash), a pane started from a plain ssh shell would run with the web UI disabled (`WEB_BASE_URL` empty) and nginx would 502. Production is single-account: `WA_ACCOUNTS` stays unset (account `default`).

Deploy procedure (after committing and pushing to `master` locally):

1. `ssh emangtop@103.30.194.149`
2. `tmux a -t pm-wa`
3. Stop the current process (Ctrl+C)
4. `git pull`
5. `export WEB_BASE_URL=https://reminder-wa.emang.top WEB_LISTEN_ADDR=:8888`
6. `go build .`
7. `./pm-wa`

Detach from tmux with Ctrl+B then `d` — this leaves the bot running. Never close the attached pane with `exit`; that kills the session and stops the bot.

Verify after every deploy: `curl -o /dev/null -w '%{http_code}' https://reminder-wa.emang.top/` must return `401` (login page, not logged in) — `502` means the env vars are missing and `./pm-wa` runs WhatsApp-only.

## Agent skills

### Issue tracker

Issues live in GitHub Issues for `danielistyo/PM-WA`, managed with the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-role triage vocabulary, each label string equal to its role name. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: root `GLOSSARY.md` + `docs/adr/`, created lazily. See `docs/agents/domain.md`.
