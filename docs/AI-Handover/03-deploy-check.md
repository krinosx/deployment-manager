# Component 3: `deploy-check` Binary

See `00-overview.md` for full context, architecture table, and cross-component
contracts (journal field names, state file) before making decisions here.

## Scope

The one-shot CLI binary that systemd's timer unit (component 4) actually invokes. Thin
orchestration layer over `gitutil`/`pipeline` (component 2) and `journallog` (component 1).

## Flow

1. Acquire a lock (`flock` on a well-known lockfile) so a timer-triggered run and a
   TUI-triggered manual run can never race. If lock can't be acquired, exit cleanly
   (log or not — open question below) rather than blocking.
2. Read `last-deployed-sha` from the state file.
3. `gitutil.RemoteHeadSHA(branch)` to get the current remote SHA — no working-tree
   changes yet.
4. Compare. If equal: nothing to do, release lock, exit 0 (whether this "no-op" case
   gets a journal entry at all, and at what priority, is an open question — see below).
5. If different: run the pipeline stages in order —
   `backup → pull → configure → make` — logging one journal entry per stage via
   `journallog.LogStage(...)`, including a `start`/`success`/`failure` status and
   duration per stage.
6. If any stage fails: stop the pipeline (don't proceed to later stages), leave
   `last-deployed-sha` unchanged, release lock, exit non-zero.
7. If all stages succeed: atomically update `last-deployed-sha` to the new SHA, release
   lock, exit 0.

## CLI flags (indicative, not finalized)

- `--dry-run` — do the check but skip actually running the pipeline; useful for testing
  the check logic and lock/state handling in isolation.
- `--force` — run the pipeline even if remote SHA == last-deployed-sha; useful for
  re-running a deploy after fixing something without needing a new commit.
- Possibly `--branch <name>` to override a config default, if branch isn't just hardcoded.

Get this binary working and manually tested as a plain CLI (no systemd involved yet)
before moving to component 4 — much easier to debug standalone.

## Dependencies

- `internal/gitutil`, `internal/pipeline` (component 2)
- `internal/journallog` (component 1)
- `github.com/coreos/go-systemd/v22/journal` (direct dependency, wrapped by journallog)

## Open questions for this session

- Exact lockfile path and whether failing to acquire the lock should itself log a
  journal entry (e.g. "another run already in progress") or just exit silently.
- Whether "checked, nothing new" (no-op) runs get a journal entry — useful for confirming
  the timer is actually firing, but adds noise to the journal if it fires often. Could
  gate this behind a `--verbose` flag or a lower log priority (`PriDebug`).
- Whether a failed stage should attempt any rollback/cleanup (e.g. of a half-pulled repo
  state) or just leave things as-is for manual inspection — currently leaning towards
  "leave as-is, it's visible in the journal, admin investigates," but not finalized.
- Config source: hardcoded paths/branch in the binary, a small config file, or CLI flags/
  env vars only? Given this runs under systemd, env vars set in the `.service` unit
  (component 4) might be the simplest option — worth deciding together with component 4.
