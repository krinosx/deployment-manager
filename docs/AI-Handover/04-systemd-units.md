# Component 4: systemd Service + Timer Units (Updated — Done)

See `00-overview.md` for full context before making decisions here. **This component is
complete, installed, and verified on the user's real Ubuntu 26.04 machine.**

## Decisions made (resolving prior open questions)

- **System-wide unit**, installed under `/etc/systemd/system/`, not a user unit. Chosen
  deliberately so the tool keeps running regardless of interactive login/session state
  — appropriate for something meant to run unattended on a server long-term.
- **Config path passed explicitly via `ExecStart`**, not a fixed default path baked into
  the Go binary. Keeps `deploy-check` itself config-path-agnostic.
- **Runs as a non-root user** (`User=krinosx` in the actual installed unit — substitute
  the real deploy/service account name in other environments), since nothing the tool
  does requires root privileges.

## Final unit files (as installed)

**Update:** as of component 6, these two files live as templates in `resources/` and are
no longer hand-installed — `scripts/install.sh` copies `deploy-check.timer` verbatim and
substitutes `User=__SERVICE_USER__` in `deploy-check.service` for the real installing
account (`$SUDO_USER`, or an explicit `SERVICE_USER=` override) via `sed`. The shape
below is unchanged, just the mechanism for getting it onto disk — see
`06-packaging-deploy.md`.

`/etc/systemd/system/deploy-check.service` (as installed, `User=` filled in by
`install.sh`):
```ini
[Unit]
Description=MUD Server deployment operator.
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/deploy-check --config /etc/deployment-manager/config.json
User=krinosx
SyslogIdentifier=deploy-check
```

`/etc/systemd/system/deploy-check.timer`:
```ini
[Unit]
Description=Run deploy-check periodically

[Timer]
OnBootSec=2min
OnUnitActiveSec=15min
Persistent=true

[Install]
WantedBy=timers.target
```

Notes on details that differ from the original draft in earlier planning:
- `Wants=network-online.target` / `After=network-online.target` were **added**, not in
  the original draft — needed since `deploy-check` does real git network operations and
  a boot-time run could otherwise race against networking not being fully up.
- `WorkingDirectory=` was **dropped** from the original draft — no longer needed since
  the config path is explicit and nothing in the code relies on relative paths or
  process working directory.
- `SyslogIdentifier=deploy-check` was **kept** — this is what makes both
  `journalctl -u deploy-check` and `journalctl -t deploy-check` work, and what
  `internal/history` (component 5) filters on via `-t deploy-check`.

## Install steps

**Original manual steps** (kept here for reference — this is essentially what
`install.sh` now does automatically):

```bash
go build -o /tmp/deploy-check ./cmd/deploy-check
sudo cp /tmp/deploy-check /usr/local/bin/deploy-check

sudo mkdir -p /etc/deployment-manager
sudo cp <local-config>.json /etc/deployment-manager/config.json

sudo cp deploy-check.service deploy-check.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now deploy-check.timer
```

**Actual current process** (component 6, used for the real game-server install):
```bash
./scripts/build_package.sh                  # dev machine, builds a tarball
scp dist/deploy-manager-*.tar.gz user@host:  # copy it over
# on the target machine:
tar xzf deploy-manager-*.tar.gz && cd deploy-manager-*/
sudo ./install.sh
```
See `06-packaging-deploy.md` for the full detail, including why binaries are built
statically (`CGO_ENABLED=0`) so no Go toolchain is needed on the target.

## Verification performed

- `systemctl list-timers deploy-check.timer` confirmed scheduling.
- `sudo systemctl start deploy-check.service` triggered a real, successful run.
- `journalctl -u deploy-check -o verbose -n 30` showed expected structured fields,
  including `DEPLOY_RUN_ID` shared across all four stages of the triggered run.

## Important operational note carried into `05-deploy-tui.md` — now resolved

Since this is a **system-wide** unit, triggering it from `deploy-tui` (running as a
regular user) via `systemctl start deploy-check.service` needed some privilege
mechanism. **Resolved**: `install.sh` installs a scoped, passwordless sudoers rule
(`/etc/sudoers.d/deploy-tui`, limited to exactly `systemctl start deploy-check.service`
for the installing user) as part of every install. `deploy-tui` shells out to
`sudo systemctl start deploy-check.service`; without the passwordless rule, `sudo` would
prompt on the TUI's controlling TTY and corrupt the bubbletea display, so this rule isn't
optional polish — the trigger-now feature doesn't work at all without it. See
`05-deploy-tui.md` and `06-packaging-deploy.md`.

## Nothing else outstanding here

No further work is planned on the unit files themselves unless requirements change
(e.g. wanting sub-15-minute reaction time, which would just mean lowering
`OnUnitActiveSec`).
