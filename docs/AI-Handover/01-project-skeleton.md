# Component 1: Project Skeleton + Config (Updated — Done)

See `00-overview.md` for full context and current architecture status before making
decisions here. **This component is complete.**

## What was built

```
deployment-manager/
  go.mod
  go.sum
  cmd/
    deploy-check/main.go
    deploy-tui/main.go
  internal/
    gitutil/
    pipeline/
    journallog/
    history/
    config/
      config.go
      config_test.go
      testdata/config.json
  resources/       <- component 6: packaging templates (unit files, config template)
  scripts/         <- component 6: build_package.sh, install.sh
```

`resources/` and `scripts/` were added for component 6 (packaging/install), not part of
this component's original scope — see `06-packaging-deploy.md`.

- Module initialized as `deploy-manager` (import paths in code use
  `deploy-manager/internal/...`).
- `internal/config/config.go` — loads a JSON config file into a `config.Config` struct
  at startup, path supplied via CLI flag (not hardcoded, not env-based). This decision
  was made deliberately, ahead of writing `deploy-check`, specifically to satisfy the
  requirement that paths and the backup-enable toggle be configurable without code
  changes.

```go
type Config struct {
	Pipeline      pipeline.Config `json:"pipeline"`
	Branch        string          `json:"branch"`
	EnableBackup  bool            `json:"enable_backup"`
	StateFilePath string          `json:"state_file_path"`
}

func Load(path string) (Config, error) { ... }
```

- `Branch` and `EnableBackup` live at the top level of `config.Config`, not nested
  inside `pipeline.Config` — see `00-overview.md`'s Config section for the reasoning
  (they're orchestration-level decisions, not filesystem details).
- `internal/config/testdata/config.json` — a real fixture file used by
  `config_test.go`'s `TestLoad`. `testdata/` is the Go convention for test fixtures;
  the toolchain ignores that directory name during normal builds.
- `journallog` was originally a bare package with no dependency; it now depends on
  `github.com/coreos/go-systemd/v22/journal` (see `03-deploy-check.md` /
  `00-overview.md` for the real-implementation details — the stub-to-real swap is done).
- `internal/history/` was added for component 5, not part of this component's original
  scope — see `05-deploy-tui.md`.

## Verified

- `go build ./...` and `go test ./...` both pass from the module root.
- `config.Load` tested against a real fixture file, including a missing-file error case.

## Nothing outstanding here

This component's original open questions (full vs. short SHA consistency, state file
location) were resolved through the config-file approach — location is fully
configurable now, not a hardcoded decision baked into this component. No further work
is anticipated on the skeleton itself; new packages (like `history`) get added here as
new components need them, following the same `internal/<name>/<name>.go` +
`<name>_test.go` pattern already established.
