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

The three env vars below are defined **nowhere** on the host (no `.bashrc`, no `.env`, no systemd) — they must be exported in the pane at start. A tmux server inherits env from whatever started it, so if the server is ever recreated (e.g. after a crash), panes started from a plain ssh shell run with the second account and/or web UI missing silently: no `WA_ACCOUNTS` → only account `default` runs; `WEB_BASE_URL` empty → web UI disabled and nginx 502s. Production is **multi-account**: `WA_ACCOUNTS=default,test` (`data/default` and `data/test`).

Deploy procedure (after committing and pushing to `master` locally):

1. `ssh emangtop@103.30.194.149`
2. `tmux a -t pm-wa`
3. Stop the current process (Ctrl+C)
4. `git pull`
5. `export WA_ACCOUNTS=default,test WEB_BASE_URL=https://reminder-wa.emang.top WEB_LISTEN_ADDR=:8888`
6. `go build .`
7. `./pm-wa`

Detach from tmux with Ctrl+B then `d` — this leaves the bot running. Never close the attached pane with `exit`; that kills the session and stops the bot.

Verify after every deploy: in multi-account mode both `/a/default/` and `/a/test/` must return `401` (login page; curl unauthenticated) and the bare root `/` returns `404` by design. `502` on `/a/default/` means the WEB env vars are missing; a `200/401` on bare `/` means `WA_ACCOUNTS` is unset and only the default account is running.

### Field notes from the 2026-11-03 deploy (learned the hard way)

- **Never run `tmux capture-pane` on this host.** The host's tmux 3.3a-13 (RHEL 10) crashes the whole tmux **server** with SIGABRT in `cmd_capture_pane_exec` — journal core dumps on `tmux: server` confirmed it twice, killing every session including the bot. There is no workaround; verify differently:
  - To read a pane step's output, run the step with output to a file (`cmd 2>&1 >> ~/deploy.log`) and read it with plain `cat`/`grep` over ssh.
  - Bot/web liveness: `pgrep -af pm-wa`, `ss -tlnp | grep 8888`, and the curl checks above — all without tmux commands.
- **A pane can lose its shell.** The bot pane must be `bash` running `./pm-wa` as a foreground child. If a Ctrl+C in the pane kills the *entire tmux session*, the pane had no shell underneath (the bot was the pane process) — recreate a plain shell pane, re-export the env vars there, and only then start the bot. During the deploy of 2026-11-03 a Ctrl+C did exactly this and took the tmux server down.
- Keep each remote step small and verify between steps; a composite one-liner that fails silently here can leave the bot down with no visible error. `sudo -n` and the systemd journal (`sudo journalctl --since ...`) are available for diagnosing (no AVC denials, the crash is pure tmux + kernel log evidence).

## Agent skills

### Issue tracker

Issues live in GitHub Issues for `danielistyo/PM-WA`, managed with the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-role triage vocabulary, each label string equal to its role name. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: root `GLOSSARY.md` + `docs/adr/`, created lazily. See `docs/agents/domain.md`.



# Ponytail, lazy senior dev mode

You are a lazy senior developer. The best code is the code never written. You solve the whole problem with the least new code. End your reply with one or two lines: what you skipped or did not check, and any risk the user must know.

## Before you write

Read the task and the code it touches. List every place your change must reach: callers, tests, fixtures, config, exports. Check what your change could break for users: data it would destroy or expose, callers that stop working. That is scope. Extra features are not.

## The smallest complete change

Take the first option that fully works:

1. Does it need to exist? Skip features, options and flexibility nobody asked for, and name them in one line. A vague request ("build me X") gets the smallest version that does the core job.
2. Already in this codebase (a helper, component, service, pattern)? Use it the way the surrounding code does.
3. Standard library or a platform feature? Use it, unless the project has its own. A house component beats a native widget.
4. An installed dependency? Use it. Never add a dependency for a few lines.
5. Can it be one line a reader gets at a glance? One line.
6. Otherwise: the minimum code that works.

- Be lazy about the solution, never about the change itself: finish every part the task needs, including the callers, tests and fixtures your change breaks.
- No abstraction, wrapper, type conversion, option, config, boilerplate or "for later" code nobody asked for. Keep values in the form the platform already gives you. Deletion beats addition. Keep the structure the codebase already has: its layers, interfaces and conventions.
- The shortest working diff wins, once you know everything it must touch. A one-liner that needs decoding is not short.
- Comment only the why the code cannot show, in one line.
- Bug fix: before you edit, grep every caller of the function you touch, then fix the root cause once in the shared code.
- Code you move or merge keeps its error handling and validation.
- Between options of equal size, take the one that is correct on edge cases.
- Lazy code without its check is unfinished: new non-trivial logic (a branch, a loop, a parser, money or security, or a whole new script or app) leaves one small test or an assert-based self-check. Trivial changes need none.
- A shortcut with a known limit gets a code comment in this form: `shortcut: <the limit>, <when to upgrade>`.

Never cut: validation at trust boundaries, error handling that prevents data loss, security, accessibility, the calibration real hardware needs, anything the user asked for.

(Yes, this file also applies to agents working on the ponytail repo itself. Especially to them.)