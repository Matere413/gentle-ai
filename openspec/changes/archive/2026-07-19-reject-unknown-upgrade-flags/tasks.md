# Tasks: Reject Unknown Upgrade Flags

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 90–150 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR / one behavior work unit |
| Delivery strategy | ask-on-risk (requested ask-always policy) |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Reject unknown pre-delimiter flags while preserving upgrade parsing; keep tests with behavior | PR 1 | `go test ./internal/app/... -run 'Test.*Upgrade'` | Build `go build -o /tmp/gga .`; run with isolated HOME: `upgrade --bad-flag` and `upgrade -x`, expect non-zero and token in stderr/output | Revert `internal/app/app.go` and `internal/app/update_test.go`; removes only this parser behavior and tests |

Dependencies: approved issue #535; existing `runUpgrade` seams; no migration or `internal/update` changes. Acceptance evidence: unknown flags fail before HOME/check/spinner/execute, supported flags remain accepted, `--` preserves dash-prefixed filters, and positional filters retain order.

## Phase 1: RED — Failing Behavioral Tests

- [x] 1.1 In `internal/app/update_test.go`, add table-driven tests for `--bad-flag`, `-x`, and another dash-prefixed token; inject HOME/check/execute seams and assert token-identifying errors, zero side effects, and empty stdout.
- [x] 1.2 Add RED cases for `--dry-run`, `-n`, `--no-backup`, combinations, ordinary `typo`, and `-- --legacy-tool`; assert accepted options and exact positional filter slices.

## Phase 2: GREEN — Minimal Parser Change

- [x] 2.1 In `internal/app/app.go`, add `upgradeUserHomeDir = os.UserHomeDir` and validate args before HOME resolution; accept only documented flags, switch on the first `--`, append post-delimiter tokens, and return an error naming every rejected flag.
- [x] 2.2 Run the focused upgrade tests and adjust only implementation details required to satisfy the RED cases; do not alter `update.CheckFiltered` or upgrade execution semantics.

## Phase 3: REFACTOR / Verification

- [x] 3.1 Refactor the local parsing loop for clarity, run `gofmt`, and preserve the one-work-unit rollback boundary.
- [x] 3.2 Verify acceptance evidence with `go test ./internal/app/...`, `go test ./...`, and `go vet ./...`; compare every spec scenario. Docker E2E is N/A because parsing is covered at the app boundary and no integration changes.
