```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:bcc798a4c7e3bf48613c65936fa63f5eac01dac8d085876c6aad1f286b82b301
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 5/5
scenarios: 6/6
test_command: go test ./internal/app/... -count=1 -run 'TestRunUpgrade_(RejectsUnknownFlagsBeforeSideEffects|PreservesSupportedFlagsAndFilters)$'
test_exit_code: 0
test_output_hash: sha256:364b36d58a597a66b8fdc2059fd299e5ad29158f1c049cb45f46e484fda5a828
build_command: go build -o /var/folders/3h/2p5xjx012bq2b7vs3ykb4yfr0000gn/T/opencode/gga-issue535-verify-refresh ./cmd/gentle-ai
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: `reject-unknown-upgrade-flags`  
**Mode**: Strict TDD / hybrid persistence  
**Native review binding**: approved lineage `review-bcc798a4c7e3bf48` is bound to this change at `sha256:9a76d59c7948148f810a8ff4d39b577e2da2bcea978985d7d42e5aec5a4aef68`. This verification only records that binding; it does not mutate review authority.

### Completeness

| Metric | Value |
|---|---:|
| Tasks total | 6 |
| Tasks complete | 6 |
| Tasks incomplete | 0 |
| Spec requirements | 5 |
| Spec scenarios | 6 |

The specification has six scenarios: two under unsupported flags, and one each for supported flags, delimiter semantics, positional filters, and identifiable validation errors. The former 5/5 scenario total was incorrect.

### Build & Tests Execution

| Command | Exit | Output SHA-256 | Result |
|---|---:|---|---|
| `go test ./internal/app/... -count=1 -run 'TestRunUpgrade_(RejectsUnknownFlagsBeforeSideEffects|PreservesSupportedFlagsAndFilters)$'` | 0 | `364b36d58a597a66b8fdc2059fd299e5ad29158f1c049cb45f46e484fda5a828` | PASS |
| `go test ./internal/app/... -count=1` | 0 | `9a9a50426f2f79540b321e4b64fda2f768e991d4a918deb9f0065dc4f0edc14a` | PASS |
| `go vet ./...` | 0 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | PASS |
| `go build -o /var/folders/3h/2p5xjx012bq2b7vs3ykb4yfr0000gn/T/opencode/gga-issue535-verify-refresh ./cmd/gentle-ai` | 0 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | PASS |
| isolated-HOME binary harness | 0 | `286a6c8a2c27793aa696e44bc456a7ae0d30153830a2a5ef326b26b01c93d5fd` | PASS |
| `go test ./... -count=1` | 1 | `76cac47b83016b9fe6eacd45d3e6511da4e2852722ad9d3c2c23c709220e4a5d` | unrelated baseline warning |
| `go test ./internal/components/communitytool -count=1 -run '^TestPiCodeGraphProbeClassifiesStalledMCPResponsesAsDeadlineExceeded$'` | 1 | `06ae2b5f80ffc4cb373c2a817b13323927481c7f87146caf1ed36b9ffed0da1e` | reproduces baseline failure |

#### Canonical verification evidence bytes

The following execution output bytes are preserved verbatim in this report; the report itself is the canonical verification-evidence preimage.

```text
focused-app output bytes:
ok  	github.com/gentleman-programming/gentle-ai/internal/app	0.434s

build output bytes:

isolated-HOME runtime output bytes:
ARG=--bad-flag EXIT=1 OUTPUT=Error: unknown upgrade flag "--bad-flag"

ARG=-x EXIT=1 OUTPUT=Error: unknown upgrade flag "-x"

HOME_ARTIFACTS=none
```

The isolated-HOME harness ran the freshly built binary with `HOME=$(mktemp -d)` and `GENTLE_AI_NO_SELF_UPDATE=1`. It asserted non-zero exits, token-bearing errors, no `Checking for updates` output, and no `.gentle-ai` artifact in that HOME.

The repository-wide suite still fails only at unchanged `internal/components/communitytool/pi_codegraph_test.go`: `tools_list` expected `MCP tools/list: read response` and received `MCP initialize: read response: context deadline exceeded`. The focused unchanged-package command reproduces the same failure, while both current app commands pass. It is therefore an unrelated baseline warning, not a candidate regression.

### Spec Compliance Matrix

| Requirement | Scenario | Passing runtime coverage | Result |
|---|---|---|---|
| Reject unsupported dash-prefixed arguments | Unknown long flag fails before side effects | `TestRunUpgrade_RejectsUnknownFlagsBeforeSideEffects/long_flag`; isolated-HOME `upgrade --dry-run --bad-flag` | ✅ COMPLIANT |
| Reject unsupported dash-prefixed arguments | Unknown short flag fails before side effects | `TestRunUpgrade_RejectsUnknownFlagsBeforeSideEffects/short_flag`; isolated-HOME `upgrade --dry-run -x` | ✅ COMPLIANT |
| Preserve supported upgrade flags | Supported flags remain accepted | `TestRunUpgrade_PreservesSupportedFlagsAndFilters`: dry run, short dry run, no backup, and combination subcases | ✅ COMPLIANT |
| Preserve delimiter semantics | Dash-prefixed filter after delimiter is positional | `TestRunUpgrade_PreservesSupportedFlagsAndFilters/dash_prefixed_filter_after_delimiter` | ✅ COMPLIANT |
| Preserve positional tool filters | Existing positional filter behavior remains unchanged | `TestRunUpgrade_PreservesSupportedFlagsAndFilters/positional_filter` | ✅ COMPLIANT |
| Report validation errors consistently | Each unknown flag is identifiable | long, short, and single-dash unit subcases; both isolated-HOME binary cases | ✅ COMPLIANT |

**Compliance summary**: **5/5 requirements and 6/6 scenarios compliant**.

### Correctness

| Requirement | Status | Notes |
|---|---|---|
| Unknown pre-delimiter flags | ✅ Implemented | `strings.HasPrefix(arg, "-")` returns `unknown upgrade flag %q` before HOME, spinner, check, or execute calls. |
| No side effects | ✅ Implemented and tested | Unit tests require zero HOME/check/execute calls and empty stdout; the isolated-HOME runtime check confirms no spinner output or HOME artifacts. |
| Supported flags | ✅ Implemented and tested | `--dry-run`, `-n`, and `--no-backup` preserve their option values. |
| Delimiter and positional compatibility | ✅ Implemented and tested | The first `--` changes mode; subsequent dash-prefixed tokens are preserved as filters; ordinary `typo` remains a filter. |
| Stable identifiable errors | ✅ Implemented and tested | Every rejected token is included in the error independently of upgrade-system availability. |

### Design Coherence

| Decision | Followed? | Notes |
|---|---|---|
| Local `afterDelimiter` parser state | ✅ Yes | The first delimiter is consumed and is not passed to the filter. |
| Narrow `upgradeUserHomeDir` seam | ✅ Yes | Initialized to `os.UserHomeDir` and injected by safety-ordering tests. |
| Validate before side effects | ✅ Yes | The validation loop precedes HOME resolution and all update/spinner/execution operations. |
| Preserve update filtering implementation | ✅ Yes | The exact filter slice remains the input to `updateCheckFiltered`; no update-package implementation changed. |
| One reversible work unit | ✅ Yes | The implementation remains limited to `internal/app/app.go` and `internal/app/update_test.go`; the rollback boundary is unchanged. |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD evidence reported | ✅ | `apply-progress.md` contains all six completed task rows. |
| All tasks have tests/evidence | ✅ | 6/6: behavioral/parser work cites `update_test.go`; refactor/verification work cites focused tests and the runtime harness. |
| RED confirmed | ✅ | The changed test file contains three rejection cases and six compatibility cases. |
| GREEN confirmed | ✅ | The focused and full app commands pass in this verification. |
| Triangulation adequate | ✅ | Three invalid tokens and six valid/delimiter/filter cases cover the six specified scenarios. |
| Safety net for modified files | ✅ | The app package safety-net command passes; the modified test file is not treated as new. |

**TDD Compliance**: 6/6 checks passed.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---:|---:|---|
| Unit | 9 table subcases | 1 | Go `testing` |
| Integration | 0 | 0 | not applicable |
| E2E | 0 | 0 | Docker E2E N/A by design |
| **Total** | **9** | **1** | |

### Changed File Coverage

Coverage was not rerun for this evidence-only refresh: the focused runtime tests, full app suite, static analysis, build, and isolated-HOME checks are the verification necessary to correct the scenario count. Coverage is informational under Strict TDD and does not establish scenario compliance.

### Assertion Quality

**Assertion quality**: ✅ All changed assertions verify real behavior. The tests call `runUpgrade`, assert token-bearing errors, exact zero side-effect counts, empty output, option values, and exact filter slices. No tautologies, ghost loops, assertion-free paths, smoke-only checks, or mock-heavy patterns were found.

### Quality Metrics

**Static analysis**: ✅ `go vet ./...` completed with exit 0 and empty output.  
**Type checking**: ✅ Go compilation is included in the passing test and build commands.  
**Formatter**: not rerun; this evidence refresh did not edit implementation or test source.

### Issues Found

**CRITICAL**: None.  
**WARNING**: The current full suite fails at the independently reproduced, unchanged communitytool MCP deadline test described above.  
**SUGGESTION**: None.

### Verdict

**PASS WITH WARNINGS** — all **5 requirements and 6 scenarios** have current passing runtime coverage. The only failing repository-wide check is the independently reproduced unrelated baseline failure.
