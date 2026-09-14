# Component 6: Packaging & Install Scripts (New — Done, used for the real game-server install)

See `00-overview.md` for full context before making decisions here. **This component is
complete and is the actual mechanism used to install `deploy-check` and `deploy-tui` onto
the production CircleMud game server** — not just a dev-machine convenience.

## Why this exists

Earlier in the project, installing meant building on the target machine (`go build ...`,
`sudo cp ...`, manually editing unit files with the real username, hand-writing a
sudoers rule) — see `04-systemd-units.md`'s original manual steps. That's fine for one
dev machine you're sitting at, but doesn't scale to "build once, install on the actual
game server," especially since:

- The game server is **resource-constrained** (the whole point of this project), so
  installing a full Go toolchain there just to build two small binaries is wasteful.
- Doing the install steps by hand on a second machine is exactly the kind of repetitive,
  error-prone process this whole project exists to eliminate for the *game* deploy — it
  didn't make sense to leave the *deploy-manager's own* deploy process manual.

So the two scripts below split cleanly: **build on the dev machine** (has Go),
**install on the target machine** (doesn't need Go).

## `scripts/build_package.sh` (dev machine only)

```bash
./scripts/build_package.sh [output-dir]   # output-dir defaults to ./dist
```

What it does:
1. Resolves a Go binary: `$GO_BIN` env override, else `go` on `PATH`, else falls back to
   the known SDK path on this dev machine
   (`/home/krinosx/go-sdk/go1.26.7/bin/go` — this fallback is dev-machine-specific and
   harmless elsewhere since `command -v go` is tried first).
2. Computes a version string from `git rev-parse --short HEAD`, appending `-dirty` if
   `git status --porcelain` isn't clean. Falls back to `"dev"` outside a git repo.
3. Builds both binaries **statically**: `CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH)`
   — see "Why static binaries" below. Output: `bin/deploy-check`, `bin/deploy-tui`.
4. Copies `resources/config.json`, `resources/deploy-check.service`,
   `resources/deploy-check.timer`, and `scripts/install.sh` into the same staging
   directory, plus a `VERSION` file.
5. Tars the whole staging directory (named `deploy-manager-<version>/`, so extracting
   doesn't dump files loose into the current directory) into
   `dist/deploy-manager-<version>.tar.gz`.

Resulting package layout (inside the tarball):
```
deploy-manager-<version>/
  VERSION
  install.sh
  bin/
    deploy-check
    deploy-tui
  config/
    config.json          <- template only, real config is never touched if it exists
  systemd/
    deploy-check.service <- User=__SERVICE_USER__ placeholder
    deploy-check.timer
```

## `scripts/install.sh` (target machine — no Go needed)

```bash
# from inside the extracted package directory:
sudo ./install.sh
```

What it does, in order:
1. Refuses to run as non-root.
2. Resolves the service account: `$SERVICE_USER` env override, else `$SUDO_USER` (the
   account that invoked `sudo`), else `logname`. **This is why you must run
   `sudo ./install.sh` while logged in as the intended service account**, not as a
   different user su'd to root — otherwise `$SUDO_USER` resolves wrong.
3. Sanity-checks all five expected package files exist (fails fast with a clear message
   if run from the wrong directory).
4. Installs both binaries to `/usr/local/bin/`.
5. Installs `config/config.json` to `/etc/deployment-manager/config.json` **only if that
   file doesn't already exist** — a real, already-configured install is never
   overwritten. On a genuinely fresh machine, this means the template lands with
   placeholder paths (`/opt/mud/...`) that **must be hand-edited** before the timer does
   anything real.
6. Templates `systemd/deploy-check.service` (substitutes `__SERVICE_USER__`) and installs
   both unit files, then `daemon-reload` and `enable --now deploy-check.timer`.
7. Generates and installs a sudoers rule at `/etc/sudoers.d/deploy-tui`:
   `<user> ALL=(root) NOPASSWD: <systemctl path> start deploy-check.service` — validated
   with `visudo -cf` on a temp file before being installed with mode 440 (refuses to
   install if validation fails, rather than risking a broken sudoers file). This is what
   lets `deploy-tui`'s `t` key work without a password prompt corrupting the TUI.

## Why static binaries (`CGO_ENABLED=0`)

This was a direct question from the user (moving from the dev machine to a real,
different game server): does the target need a Go runtime, or just the binaries?

Answer: **just the binaries** — Go compiles to self-contained executables, no separate
runtime to ship. But there's a real catch that was verified empirically, not assumed:
building normally (`CGO_ENABLED=1`, the Go default) produces a binary **dynamically
linked against glibc** (`libc.so.6`), even though this codebase has no `.go` files using
`import "C"` anywhere. The cause: `go-systemd/journal.Send` uses `net.Dial` for the
journal socket, and Go's `net` package pulls in the cgo-based DNS resolver on Linux
whenever `CGO_ENABLED=1`, regardless of whether that code path is actually exercised at
runtime. A dynamically-linked binary built on one machine can fail to run on another with
an older glibc (`GLIBC_2.XX not found`).

Fix: build with `CGO_ENABLED=0`. Verified: `ldd` reports "not a dynamic executable" and
the binary still runs correctly (journal logging here never needed cgo in practice).
`build_package.sh` always builds this way — this is not optional/configurable.

The other half of "different machine" — kernel version — turned out to be a non-issue:
Linux keeps its syscall ABI backward-compatible, and Go's minimum kernel floor is far
below anything a current Ubuntu ships, so a different kernel between dev machine and game
server was never actually a risk. Architecture (amd64 vs. arm64) is the one thing that
does matter and isn't checked automatically — `build_package.sh` builds for the dev
machine's own `GOARCH`, so cross-arch deploys would need an explicit `GOARCH` override
(not currently plumbed through as a script argument; would need adding if the game server
is ever a different architecture than the dev machine).

## Known caveats / things to remember on a fresh install

- **Config is never auto-populated with real values.** After a fresh install, you must
  manually edit `/etc/deployment-manager/config.json` with the real `repo_dir`,
  `src_dir`, `lib_dir`, `backup_dir`, and `branch` before the timer fires for real —
  otherwise the first run fails against placeholder `/opt/mud/...` paths.
- **First run on a brand-new machine deploys immediately, not a no-op.** With no
  `last-deployed-sha` state file yet, `deploy-check` treats "no prior deploy" as
  different from remote HEAD, so the very first check (whether from the timer's
  `OnBootSec=2min` or a manual trigger) attempts a real deploy rather than being a no-op.
  If you want to verify the config without side effects first, run manually with
  `--dry-run` before letting the timer or `deploy-tui`'s trigger touch it.
- **The sudoers rule is scoped to whoever ran `install.sh`.** Re-running install as a
  different user on the same machine adds/overwrites the rule for that user, but doesn't
  remove a stale rule for a previous one (not currently handled — a minor cleanup item if
  the service account is ever changed on an existing install).
- `dist/` is gitignored — package tarballs are build artifacts, not source.

## Verified

- Built a real package on the dev machine, extracted it to a scratch directory, and
  confirmed: binaries are genuinely static (`ldd` → "not a dynamic executable"),
  `systemd/deploy-check.service` carries the `__SERVICE_USER__` placeholder correctly,
  `install.sh` is present and executable, `VERSION` is recorded.
- Ran `sudo ./install.sh` for real on the dev machine (re-installing over an existing
  config) — confirmed it left the existing config untouched, correctly templated and
  reinstalled the unit files, re-validated and reinstalled the sudoers rule, and that
  `deploy-tui`'s trigger-now and `deploy-check`'s no-op logging both worked immediately
  afterward against the newly installed binaries.
- **Used for the actual production install on the CircleMud game server** — the intended
  real-world use case for this whole component, confirmed working by the user.

## Nothing else outstanding here

No further work is currently planned on the packaging scripts themselves. The
cross-architecture gap noted above (no `GOARCH` override wired through
`build_package.sh`) is the one known limitation, and only matters if the game server is
ever a different CPU architecture than the dev machine — not the case currently.
