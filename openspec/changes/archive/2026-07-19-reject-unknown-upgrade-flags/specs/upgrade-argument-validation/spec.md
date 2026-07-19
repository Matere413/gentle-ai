# Upgrade Argument Validation Specification

## Purpose

Define the externally observable argument contract for `gentle-ai upgrade` so unsupported flags fail safely while supported flags and positional filters retain their existing behavior.

## Requirements

### Requirement: Reject unsupported dash-prefixed arguments

The `upgrade` command MUST reject every unrecognized argument beginning with `-` or `--` while it is parsing flags. It MUST return a non-zero error before performing update checks, emitting spinner output, resolving upgrade work, or executing an upgrade. The error MUST identify the offending argument or otherwise make the unknown-flag cause unambiguous, without requiring incidental wording or formatting.

#### Scenario: Unknown long flag fails before side effects

- GIVEN `upgrade` receives `--bad-flag` before the delimiter
- WHEN the command is invoked
- THEN it returns a non-zero error identifying `--bad-flag` as unsupported
- AND no update check, spinner output, or upgrade execution occurs

#### Scenario: Unknown short flag fails before side effects

- GIVEN `upgrade` receives `-x` before the delimiter
- WHEN the command is invoked
- THEN it returns a non-zero error identifying `-x` as unsupported
- AND no update check, spinner output, or upgrade execution occurs

### Requirement: Preserve supported upgrade flags

The command MUST continue accepting `--dry-run`, `-n`, and `--no-backup` with their existing meanings and MUST NOT report them as unknown arguments.

#### Scenario: Supported flags remain accepted

- GIVEN `upgrade` receives any supported flag, alone or in a valid combination
- WHEN the command is invoked
- THEN argument validation succeeds and the selected flag behavior is preserved

### Requirement: Preserve delimiter semantics

The command MUST treat the first standalone `--` as the end-of-flags delimiter. After it, dash-prefixed tokens MUST be treated as positional tool filters rather than flags and MUST NOT be rejected as unknown flags.

#### Scenario: Dash-prefixed filter after delimiter is positional

- GIVEN `upgrade` receives `--` followed by a tool filter such as `--legacy-tool`
- WHEN the command is invoked
- THEN the token is passed as a positional filter
- AND it is not rejected as an unknown flag

### Requirement: Preserve positional tool filters

The command MUST continue accepting ordinary positional tool filters before or after supported flags, and MUST preserve existing behavior for unknown tool names. Argument validation MUST NOT validate or reinterpret tool names.

#### Scenario: Existing positional filter behavior remains unchanged

- GIVEN `upgrade` receives a positional filter such as `typo`
- WHEN the command is invoked
- THEN validation succeeds and the filter is handled by the existing upgrade filtering behavior

### Requirement: Report validation errors consistently

For every rejected unknown flag, the command MUST expose a stable error classification or identifiable offending token so callers and tests can distinguish unknown-flag failures from ordinary upgrade failures. The specification does not require exact incidental text, punctuation, ordering, or presentation styling.

#### Scenario: Each unknown flag is identifiable

- GIVEN any unsupported dash-prefixed token is supplied before `--`
- WHEN the command is invoked
- THEN the non-zero result identifies that token as the validation failure
- AND the result does not depend on upgrade-system availability
