# Apply Progress: Reject Unknown Upgrade Flags

## Status

All 6 of 6 tasks are complete in Strict TDD mode. Delivery is one approved single-PR work unit; no chain strategy or size exception applies.

## Completed Tasks

- [x] 1.1 Unknown pre-delimiter flag RED tests with zero-side-effect assertions.
- [x] 1.2 Supported-flag, positional-filter, and delimiter RED tests.
- [x] 2.1 Early delimiter-aware validation and HOME seam.
- [x] 2.2 Focused upgrade tests green without changing update filtering or execution semantics.
- [x] 3.1 Local parser clarity review and `gofmt`.
- [x] 3.2 Acceptance verification and static analysis.

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1 | `internal/app/update_test.go` | Unit | `go test ./internal/app/...` — PASS | Added unknown long, short, and single-dash cases before production change; focused test failed to compile because `upgradeUserHomeDir` did not exist | `go test ./internal/app/... -run 'TestRunUpgrade_(RejectsUnknownFlagsBeforeSideEffects|PreservesSupportedFlagsAndFilters)$'` — PASS | 3 distinct rejected tokens; every case asserts token, no HOME/check/execute calls, and empty stdout | `gofmt` then focused test — PASS |
| 1.2 | `internal/app/update_test.go` | Unit | `go test ./internal/app/...` — PASS | Added supported parsing cases before production change in the same RED run | Same focused command — PASS | 6 cases: three supported flag forms, combination, ordinary positional filter, and post-delimiter dash-prefixed filter | `gofmt` then focused test — PASS |
| 2.1 | `internal/app/app.go` | Unit | App package safety net — PASS | Tests referenced the nonexistent HOME seam and failed before implementation | Focused command — PASS after adding the seam and delimiter-aware parser | Covered unknown and valid parsing branches through 9 table cases | Parser retained as one clear local loop; `gofmt` and focused test — PASS |
| 2.2 | `internal/app/app.go` | Unit | App package safety net — PASS | Same RED evidence as 1.1–1.2 | Focused command — PASS | All requested flag, filter, and delimiter scenarios exercised | No further change needed after formatting |
| 3.1 | `internal/app/app.go` | Unit | Focused tests — PASS | N/A — formatting/refactor task | Focused command — PASS | N/A — no behavior change | `gofmt -w internal/app/app.go internal/app/update_test.go`; focused test — PASS |
| 3.2 | `internal/app` | Unit / runtime harness | Focused tests — PASS | N/A — verification task | `go test ./internal/app/...` — PASS | Full acceptance scenarios covered by focused table tests and isolated-binary harness | No production refactor in verification |

## Work Unit Evidence

| Evidence | Result |
|---|---|
| Focused test command and exact result | `go test ./internal/app/... -run 'TestRunUpgrade_(RejectsUnknownFlagsBeforeSideEffects|PreservesSupportedFlagsAndFilters)$'` — PASS (exit 0); `go test ./internal/app/...` — PASS (exit 0). |
| Runtime harness command/scenario and exact result | `go build -o /var/folders/3h/2p5xjx012bq2b7vs3ykb4yfr0000gn/T/opencode/gga-issue-535 ./cmd/gentle-ai`; with an isolated `mktemp -d` HOME, `gga-issue-535 upgrade --dry-run --bad-flag` and `gga-issue-535 upgrade --dry-run -x` each emitted `Error: unknown upgrade flag "…"` and exited non-zero; harness assertion passed (exit 0). No spinner/update output preceded the errors. |
| Static analysis | `go vet ./...` — PASS (exit 0). |
| Full suite | `go test ./...` — FAIL only in pre-existing/unrelated `internal/components/communitytool`: `TestPiCodeGraphProbeClassifiesStalledMCPResponsesAsDeadlineExceeded/tools_list` expected phase `MCP tools/list: read response` but received `MCP initialize: read response: context deadline exceeded`. `internal/app` passed. Earlier cached baseline also noted failures in `internal/app` and `internal/cli`; this run did not reproduce those. |
| Rollback boundary | Revert `internal/app/app.go` and `internal/app/update_test.go`; this removes only unknown pre-delimiter flag rejection, the HOME seam, and its tests without changing update filtering or execution. |
| Docker E2E | N/A — parsing is fully exercised at the application boundary and this change introduces no integration boundary. |

## Changed-Line Count

`git diff --numstat` for production and test files: 122 changed lines (120 additions, 2 deletions): `internal/app/app.go` 13 additions/2 deletions; `internal/app/update_test.go` 107 additions. This is within the 400-line review budget.

## Design Deviations

None — the implementation uses the specified local `afterDelimiter` parser state and `upgradeUserHomeDir` seam.
