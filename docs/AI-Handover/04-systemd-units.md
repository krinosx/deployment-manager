# Component 4: systemd Service + Timer Units

See `00-overview.md` for full context, architecture table, and cross-component
contracts before making decisions here.

## Scope

Plain systemd unit files that schedule `deploy-check` (component 3) to run periodically,
without any resident daemon process. This is the core of the "no resident daemon"
architecture decision — systemd itself is already resident, so idle memory cost is zero.

## Draft units (discussed so far, not finalized)

```ini
# /etc/systemd/system/deploy-check.service
[Unit]
Description=Check for new MUD version and deploy if found

[Service]
Type=oneshot
ExecStart=/usr/local/bin/deploy-check
User=youruser
WorkingDirectory=/path/to/mud/project
SyslogIdentifier=deploy-check
```

```ini
# /etc/systemd/system/deploy-check.timer
[Unit]
Description=Run deploy-check periodically

[Timer]
OnBootSec=2min
OnUnitActiveSec=15min
Persistent=true

[Install]
WantedBy=timers.target
```

- `Type=oneshot` matches the "runs and exits" model — no `Type=simple`/resident process.
- `SyslogIdentifier=deploy-check` guarantees a stable tag for
  `journalctl -t deploy-check` regardless of how the unit itself is named, and is what
  the TUI (component 5) will filter on.
- `Persistent=true` on the timer means a missed run (box was off) fires once at next
  boot instead of being silently skipped — decided as a "costs nothing, keep it."
- `OnUnitActiveSec=15min` is a placeholder interval — actual value not finalized.

## Open questions for this session

- What user should the service run as? Needs write access to the project directory,
  the `lib/` backup location, and the state file path (component 1/3) — but shouldn't
  be root if avoidable. This decision feeds back into component 1's choice of state
  file location (`/var/lib/deploy-manager/` vs. somewhere under the project directory).
- Should config (branch name, project paths) be passed via `Environment=`/`EnvironmentFile=`
  in the `.service` unit, or should `deploy-check` read a config file/have paths hardcoded?
  Flagged as an open question in component 3's doc too — decide together.
- System-level unit (`/etc/systemd/system/`) vs. user-level unit (`~/.config/systemd/user/`,
  run via `systemctl --user`) — user units avoid needing root to install/manage, but only
  run while the user has an active session unless lingering is enabled
  (`loginctl enable-linger`). Worth deciding based on how the box is administered.
- Actual interval for `OnUnitActiveSec` — depends on how quickly the user wants new
  commits picked up vs. avoiding unnecessary `git ls-remote` calls.
- Whether `deploy-tui`'s "trigger now" (component 5) should call
  `systemctl start deploy-check.service` (requires the TUI's invoking user to have
  permission to start the unit — may need a polkit rule or running the TUI as the same
  user/group) — worth resolving here since it constrains component 5's implementation.
