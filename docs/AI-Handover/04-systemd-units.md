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

`/etc/systemd/system/deploy-check.service`:
```ini
[Unit]
Description=Check for new CircleMud version and deploy if found
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/deploy-check --config /etc/deploy-manager/config.json
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

## Install steps (as actually performed)

```bash
go build -o /tmp/deploy-check ./cmd/deploy-check
sudo cp /tmp/deploy-check /usr/local/bin/deploy-check

sudo mkdir -p /etc/deploy-manager
sudo cp <local-config>.json /etc/deploy-manager/config.json

sudo cp deploy-check.service deploy-check.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now deploy-check.timer
```

## Verification performed

- `systemctl list-timers deploy-check.timer` confirmed scheduling.
- `sudo systemctl start deploy-check.service` triggered a real, successful run.
- `journalctl -u deploy-check -o verbose -n 30` showed expected structured fields,
  including `DEPLOY_RUN_ID` shared across all four stages of the triggered run.

## Important operational note carried into `05-deploy-tui.md`

Since this is a **system-wide** unit, triggering it from `deploy-tui` (running as a
regular user) via `systemctl start deploy-check.service` will require some privilege
mechanism — sudo, a polkit rule, or running the TUI itself with elevated rights. This
was a hypothetical in the original design doc; it's now a concrete blocker to resolve
before `deploy-tui`'s "trigger now" feature can work as designed. See
`05-deploy-tui.md`.

## Nothing else outstanding here

No further work is planned on the unit files themselves unless requirements change
(e.g. wanting sub-15-minute reaction time, which would just mean lowering
`OnUnitActiveSec`).
