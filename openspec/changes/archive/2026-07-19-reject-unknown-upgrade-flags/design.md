# Design: Reject Unknown Upgrade Flags

## Technical Approach

Validate `runUpgrade` arguments before resolving HOME, constructing a spinner, checking updates, or executing upgrades. Keep the current hand-written parser: recognize `--dry-run`/`-n` and `--no-backup`; consume the first standalone `--`; append every later token (including dash-prefixed ones) to `toolFilter`; reject every other dash-prefixed token with an error that includes it. This implements `upgrade-argument-validation` without changing `update.CheckFiltered` behavior.

## Architecture Decisions

| Option | Tradeoff | Decision |
|---|---|---|
| Extend the `runUpgrade` loop with an `afterDelimiter` boolean | Small, local parsing state | Chosen: preserves current ordering and positional filtering. |
| Use `flag.FlagSet` or a shared parser | Changes error/output and positional semantics beyond the issue | Rejected. |
| Add a narrow `upgradeUserHomeDir` package seam initialized to `os.UserHomeDir` | One test-only indirection | Chosen: proves invalid input cannot resolve HOME. |
| Test only returned errors | Cannot prove safety ordering | Rejected: tests also count HOME, check, and execution calls and inspect output. |

## Data Flow

```text
args -> runUpgrade validation -> error (unknown pre-delimiter flag)
                              -> HOME -> profile -> spinner/check -> execute
```

`--` changes validation mode only; it is not included in `toolFilter`. Ordinary positional tokens retain their order. `--legacy-tool` after the delimiter is passed unchanged to `updateCheckFiltered`.

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/app/app.go` | Modify | Add the narrow HOME seam and local delimiter-aware validation at `runUpgrade`. |
| `internal/app/update_test.go` | Modify | Add table-driven RED-first tests for validation, preserved arguments, and zero side effects. |

## Interfaces / Contracts

No exported API changes. Internal seam:

```go
var upgradeUserHomeDir = os.UserHomeDir
```

Before the first `--`, accepted flags are `--dry-run`, `-n`, and `--no-backup`. Any other `strings.HasPrefix(arg, "-")` returns `fmt.Errorf("unknown upgrade flag %q", arg)` (or equivalent stable error identifying `arg`). After `--`, all tokens are positional filters.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit (RED first) | `--bad-flag`, `-x`, and another unknown dash token | Table-driven `runUpgrade` tests; replace `upgradeUserHomeDir`, `updateCheckFiltered`, and `upgradeExecuteWithOptions`; assert error names token, all call counts are zero, and `stdout` is empty. |
| Unit | `--dry-run`, `-n`, `--no-backup`, combinations, ordinary `typo`, and `-- --legacy-tool` | Table-driven mocked success path; assert captured dry-run/backup options and exact filter slice. |
| Regression | Existing app package behavior | Run `go test ./internal/app/...`, then `go test ./...` and `go vet ./...`. Docker E2E is N/A: parsing is fully covered at the application boundary and makes no integration change. |

## Threat Matrix

The CLI-to-upgrade process boundary is considered; the matrix has no applicable listed rows because this change neither classifies executables nor alters Git, commit, push, or PR commands.

| Boundary | Applicability | Design response / planned RED tests |
|---|---|---|
| Documentation-like paths | N/A — no file classification | None. |
| Git repository selection | N/A — no repository selection | None. |
| Commit state | N/A — no Git mutation | None. |
| Push state | N/A — no push behavior | None. |
| PR commands | N/A — no PR automation | None. |

## Migration / Rollout

No migration required. Deliver one `fix(app): reject unknown upgrade flags` work unit with its tests, well below the 400-line budget; rollback is reverting only the two listed files.

## Open Questions

None.
