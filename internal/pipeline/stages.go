package pipeline

type Stage string

const (
	StagePrepare  Stage = "prepare"
	StageApply    Stage = "apply"
	StageRollback Stage = "rollback"
)

type Step interface {
	ID() string
	Run() error
}

type RollbackStep interface {
	Step
	Rollback() error
}

// FailurePolicy controls how the runner behaves when a step fails.
type FailurePolicy int

const (
	// StopOnError stops execution at the first failed step (default).
	StopOnError FailurePolicy = iota
	// ContinueOnError continues executing remaining steps, collecting all errors.
	ContinueOnError
)

// ProgressEvent is emitted by the runner as each step starts and completes.
type ProgressEvent struct {
	StepID string
	Stage  Stage
	Status StepStatus
	Err    error

	// Command carries an optional typed command-progress payload for live
	// per-command observation during multi-command agent install steps. It is
	// nil for ordinary step lifecycle events; the executor boundary sets it
	// when emitting START/SUCCEEDED/FAILED events for individual commands. The
	// transport is best-effort: consumers MUST reconcile against the enclosing
	// pipeline terminal result and MUST NOT rely on every intermediate event
	// being delivered.
	Command *CommandProgressEvent
}

// CommandProgressStatus is the lifecycle status of a single command within a
// multi-command agent install step. It is distinct from StepStatus because a
// command is a sub-unit of a step, not a step itself.
type CommandProgressStatus string

const (
	CommandProgressStarted   CommandProgressStatus = "started"
	CommandProgressSucceeded CommandProgressStatus = "succeeded"
	CommandProgressFailed    CommandProgressStatus = "failed"
)

// CommandProgressEvent is the typed payload for live, per-command progress
// during a multi-command agent install step. The DisplayName is the only
// user-facing label and MUST be bounded by the producer (safe allowlist or
// bounded fallback); raw argv is never placed here. Current is 1-indexed.
type CommandProgressEvent struct {
	StepID      string
	AgentID     string
	DisplayName string
	Current     int
	Total       int
	Status      CommandProgressStatus
}

// ProgressFunc is a callback invoked for every step lifecycle event.
type ProgressFunc func(ProgressEvent)

type StagePlan struct {
	Prepare []Step
	Apply   []Step
}
