# Component 1: Project Skeleton

See `00-overview.md` for full context, architecture table, and cross-component
contracts (journal field names, state file path, non-goals) before making decisions here.

## Scope

Set up the Go module and package layout that every other component builds on. No
business logic yet — just structure and shared low-level helpers.

## Proposed layout

```
deploy-manager/
  go.mod
  cmd/
    deploy-check/
      main.go
    deploy-tui/
      main.go
  internal/
    gitutil/       # git ls-remote / pull wrappers (component 2)
    pipeline/       # backup / configure / make stage logic (component 2)
    journallog/      # thin wrapper around go-systemd/journal (used by 3 and 5)
```

## Responsibilities of this component

- `go.mod` / module name, Go version pin.
- `internal/journallog`: a small helper package both `deploy-check` and (indirectly, via
  parsing) `deploy-tui` rely on for consistent field names. Should expose something like:
  ```go
  func LogStage(stage string, status string, sha string, durationMs int64, extra map[string]string)
  ```
  so call sites in `pipeline`/`deploy-check` never hardcode field name strings directly.
- Decide and hardcode the field name constants here (`DEPLOY_STAGE`, `DEPLOY_STATUS`,
  `DEPLOY_SHA`, `DEPLOY_DURATION_MS`) as Go constants, since these are the contract with
  component 5.
- Decide the state file path/format for `last-deployed-sha` (single file, atomic write
  via temp file + rename) and put a minimal read/write helper somewhere sensible —
  either its own tiny `internal/state` package or folded into `journallog`'s neighbor
  if it turns out trivial enough. (Open question — pick whichever keeps `pipeline` and
  `deploy-check` cleanest; not a hard requirement to have a separate package.)

## Explicitly out of scope here

- Actual git/build logic → component 2.
- Actual CLI flow (`--dry-run`, `--force`, locking) → component 3.
- systemd unit content → component 4.
- TUI code → component 5.

## Open questions for this session

- Full vs short SHA stored/logged consistently across the codebase — pick one and apply
  everywhere (affects both the state file and the `DEPLOY_SHA` journal field).
- Where does the state file live — `/var/lib/deploy-manager/` (needs directory creation
  logic, likely at install/systemd-unit time) vs. somewhere under the project directory
  the user already controls. Depends partly on what user/permissions the systemd service
  runs as (see component 4).
