package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func installProgressModel() Model {
	m := NewModel(system.DetectionResult{}, "test")
	m.Screen = ScreenInstalling
	m.pipelineRunning = true
	m.Progress = NewProgressState([]string{"agent:pi", "next"})
	m.Progress.Start(0)
	return m
}

func commandEvent(stepID string, current int, status pipeline.CommandProgressStatus) pipeline.CommandProgressEvent {
	return pipeline.CommandProgressEvent{StepID: stepID, Current: current, Total: 3, DisplayName: "Install Pi", Status: status}
}

func updateCommand(m Model, event pipeline.CommandProgressEvent) (Model, tea.Cmd) {
	updated, cmd := m.Update(pipelineProgressMsg{event: pipeline.ProgressEvent{StepID: event.StepID, Command: &event}, generation: m.commandProgressGeneration})
	return updated.(Model), cmd
}

func updateBridge(m Model, event pipeline.ProgressEvent) (Model, tea.Cmd) {
	updated, cmd := m.Update(pipelineProgressMsg{event: event, generation: m.commandProgressGeneration})
	return updated.(Model), cmd
}

func TestStartInstallingBridgeRoutesStepAndCommandEventsInOrder(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "test")
	m.DependencyPlan = planner.ResolvedPlan{Agents: []model.AgentID{model.AgentPi}}
	ready := make(chan pipeline.ProgressFunc)
	resume := make(chan struct{})
	m.ExecuteFn = func(_ model.Selection, _ planner.ResolvedPlan, _ system.DetectionResult, progress pipeline.ProgressFunc) pipeline.ExecutionResult {
		ready <- progress
		<-resume
		return pipeline.ExecutionResult{}
	}

	started, cmd := m.startInstalling()
	batch := cmd().(tea.BatchMsg)
	done := make(chan tea.Msg, 1)
	go func() { done <- batch[1]() }()
	progress := <-ready
	model := started.(Model)
	listener := batch[2]

	sequence := []pipeline.ProgressEvent{
		{StepID: "prepare:check-dependencies", Status: pipeline.StepStatusRunning},
		{StepID: "prepare:check-dependencies", Status: pipeline.StepStatusSucceeded},
		{StepID: "agent:pi", Status: pipeline.StepStatusRunning},
		{StepID: "agent:pi", Command: &pipeline.CommandProgressEvent{StepID: "agent:pi", Current: 1, Total: 2, DisplayName: "Install Pi", Status: pipeline.CommandProgressStarted}},
		{StepID: "agent:pi", Command: &pipeline.CommandProgressEvent{StepID: "agent:pi", Current: 1, Total: 2, DisplayName: "Install Pi", Status: pipeline.CommandProgressSucceeded}},
		{StepID: "agent:pi", Command: &pipeline.CommandProgressEvent{StepID: "agent:pi", Current: 2, Total: 2, DisplayName: "Configure Pi", Status: pipeline.CommandProgressSucceeded}},
		{StepID: "agent:pi", Status: pipeline.StepStatusSucceeded},
	}
	for _, event := range sequence {
		progress(event)
		msg := listener()
		model, listener = updateBridge(model, msg.(pipelineProgressMsg).event)
	}

	if model.Progress.Current != 4 || model.Progress.Items[3].Status != string(pipeline.StepStatusSucceeded) {
		t.Fatalf("step lifecycle did not advance progress: %+v", model.Progress)
	}
	if model.CommandProgress != (CommandProgressState{}) {
		t.Fatalf("step terminal retained command progress: %+v", model.CommandProgress)
	}
	close(resume)
	updated, _ := model.Update(<-done)
	if state := updated.(Model); state.CommandProgress != (CommandProgressState{}) || state.pipelineRunning {
		t.Fatalf("pipeline completion did not reconcile bridge state: %+v", state)
	}
}

func TestStartInstallingBridgeClearsFailedStepAndIgnoresLateCommand(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "test")
	m.DependencyPlan = planner.ResolvedPlan{Agents: []model.AgentID{model.AgentPi}}
	ready := make(chan pipeline.ProgressFunc)
	resume := make(chan struct{})
	m.ExecuteFn = func(_ model.Selection, _ planner.ResolvedPlan, _ system.DetectionResult, progress pipeline.ProgressFunc) pipeline.ExecutionResult {
		ready <- progress
		<-resume
		return pipeline.ExecutionResult{}
	}

	started, cmd := m.startInstalling()
	batch := cmd().(tea.BatchMsg)
	done := make(chan tea.Msg, 1)
	go func() { done <- batch[1]() }()
	progress := <-ready
	state := started.(Model)
	listener := batch[2]
	for _, event := range []pipeline.ProgressEvent{
		{StepID: "agent:pi", Status: pipeline.StepStatusRunning},
		{StepID: "agent:pi", Command: &pipeline.CommandProgressEvent{StepID: "agent:pi", Current: 2, Total: 3, DisplayName: "Configure Pi", Status: pipeline.CommandProgressStarted}},
		{StepID: "agent:pi", Command: &pipeline.CommandProgressEvent{StepID: "agent:pi", Current: 2, Total: 3, DisplayName: "Configure Pi", Status: pipeline.CommandProgressFailed}},
		{StepID: "agent:pi", Status: pipeline.StepStatusFailed},
	} {
		progress(event)
		message := listener().(pipelineProgressMsg)
		state, listener = updateBridge(state, message.event)
	}
	if state.Progress.Items[3].Status != string(pipeline.StepStatusFailed) || state.CommandProgress != (CommandProgressState{}) {
		t.Fatalf("failed step did not become authoritative: progress=%+v command=%+v", state.Progress, state.CommandProgress)
	}

	late := pipeline.CommandProgressEvent{StepID: "agent:pi", Current: 3, Total: 3, DisplayName: "Late", Status: pipeline.CommandProgressStarted}
	state, _ = updateBridge(state, pipeline.ProgressEvent{StepID: late.StepID, Command: &late})
	if state.CommandProgress != (CommandProgressState{}) {
		t.Fatalf("late command mutated terminal step: %+v", state.CommandProgress)
	}
	close(resume)
	updated, _ := state.Update(<-done)
	if reconciled := updated.(Model); reconciled.CommandProgress != (CommandProgressState{}) {
		t.Fatalf("pipeline done retained failed command state: %+v", reconciled.CommandProgress)
	}
}

func TestCommandProgressTransportDoesNotBlockAndResubscribes(t *testing.T) {
	m := installProgressModel()
	m = m.startCommandProgress()
	finished := make(chan struct{})
	go func() {
		for i := 0; i < commandProgressBuffer+1; i++ {
			event := commandEvent("agent:pi", 1, pipeline.CommandProgressStarted)
			m.pipelineProgress(pipeline.ProgressEvent{StepID: event.StepID, Command: &event})
		}
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("non-consuming consumer blocked producer")
	}
	if len(m.commandProgressEvents) != commandProgressBuffer {
		t.Fatalf("buffered events = %d, want %d", len(m.commandProgressEvents), commandProgressBuffer)
	}
	updated, _ := m.Update(PipelineDoneMsg{})
	m = updated.(Model)
	if m.CommandProgress != (CommandProgressState{}) {
		t.Fatalf("dropped events survived reconciliation: %+v", m.CommandProgress)
	}

	m = installProgressModel().startCommandProgress()
	m.commandProgressEvents = make(chan pipeline.ProgressEvent, 2)
	firstEvent := commandEvent("agent:pi", 1, pipeline.CommandProgressStarted)
	secondEvent := commandEvent("agent:pi", 2, pipeline.CommandProgressSucceeded)
	m.pipelineProgress(pipeline.ProgressEvent{StepID: firstEvent.StepID, Command: &firstEvent})
	m.pipelineProgress(pipeline.ProgressEvent{StepID: secondEvent.StepID, Command: &secondEvent})
	cmd := m.listenForCommandProgress()
	first := cmd().(pipelineProgressMsg)
	updated, cmd = m.Update(first)
	m = updated.(Model)
	if m.CommandProgress.Current != 1 || cmd == nil {
		t.Fatalf("first event = %+v, listener = %v", m.CommandProgress, cmd != nil)
	}
	second := cmd().(pipelineProgressMsg)
	updated, _ = m.Update(second)
	m = updated.(Model)
	if m.CommandProgress.Current != 2 || m.CommandProgress.Completed != 2 {
		t.Fatalf("second event = %+v", m.CommandProgress)
	}
}

func TestCommandProgressRejectsWrongOrTerminalStepAndCleansPipeline(t *testing.T) {
	m := installProgressModel().startCommandProgress()
	m, _ = updateCommand(m, commandEvent("agent:pi", 2, pipeline.CommandProgressSucceeded))
	m, _ = updateCommand(m, commandEvent("other", 3, pipeline.CommandProgressStarted))
	if m.CommandProgress.StepID != "agent:pi" || m.CommandProgress.LastStatus != pipeline.CommandProgressSucceeded {
		t.Fatalf("wrong step mutated state: %+v", m.CommandProgress)
	}

	updated, _ := m.Update(StepProgressMsg{StepID: "agent:pi", Status: pipeline.StepStatusSucceeded})
	m = updated.(Model)
	if m.CommandProgress.StepID != "" {
		t.Fatalf("terminal step retained command state: %+v", m.CommandProgress)
	}
	m, _ = updateCommand(m, commandEvent("agent:pi", 3, pipeline.CommandProgressStarted))
	if m.CommandProgress.StepID != "" {
		t.Fatalf("late event mutated terminal step: %+v", m.CommandProgress)
	}

	updated, _ = m.Update(PipelineDoneMsg{})
	m = updated.(Model)
	if m.CommandProgress.StepID != "" || m.commandProgressEvents != nil || m.pipelineRunning {
		t.Fatalf("pipeline cleanup = %+v, events=%v, running=%v", m.CommandProgress, m.commandProgressEvents, m.pipelineRunning)
	}
}

func TestCommandProgressReconcilesDropsOutOfOrderAndStaleGeneration(t *testing.T) {
	m := installProgressModel().startCommandProgress()
	m, _ = updateCommand(m, commandEvent("agent:pi", 3, pipeline.CommandProgressSucceeded))
	m, _ = updateCommand(m, commandEvent("agent:pi", 1, pipeline.CommandProgressStarted))
	if m.CommandProgress.Current != 3 || m.CommandProgress.Completed != 3 || m.CommandProgress.LastStatus != pipeline.CommandProgressSucceeded {
		t.Fatalf("replayed start regressed state: %+v", m.CommandProgress)
	}

	staleEvent := commandEvent("agent:pi", 2, pipeline.CommandProgressStarted)
	stale := pipelineProgressMsg{event: pipeline.ProgressEvent{StepID: staleEvent.StepID, Command: &staleEvent}, generation: m.commandProgressGeneration - 1}
	updated, _ := m.Update(stale)
	m = updated.(Model)
	if m.CommandProgress.Current != 3 || m.CommandProgress.LastStatus != pipeline.CommandProgressSucceeded {
		t.Fatalf("stale generation mutated state: %+v", m.CommandProgress)
	}

	updated, _ = m.Update(PipelineDoneMsg{})
	m = updated.(Model)
	if m.CommandProgress != (CommandProgressState{}) {
		t.Fatalf("authoritative result left state: %+v", m.CommandProgress)
	}
}

func TestPipelineDoneDoesNotCloseCompletedCommandListenerTwice(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "test")
	m.ExecuteFn = func(_ model.Selection, _ planner.ResolvedPlan, _ system.DetectionResult, _ pipeline.ProgressFunc) pipeline.ExecutionResult {
		return pipeline.ExecutionResult{}
	}
	started, cmd := m.startInstalling()
	batch := cmd().(tea.BatchMsg)
	done := batch[1]()
	if listener := batch[2](); listener != nil {
		t.Fatalf("listener message = %T, want nil after execution closes it", listener)
	}
	updated, _ := started.Update(done)
	if state := updated.(Model); state.commandProgressDone != nil {
		t.Fatal("pipeline completion retained command listener lifecycle")
	}
}

func TestInstallViewProgressUsesCompletedCommandsMonotonically(t *testing.T) {
	m := installProgressModel().startCommandProgress()

	m, _ = updateCommand(m, commandEvent("agent:pi", 2, pipeline.CommandProgressStarted))
	started := m.Progress.ViewModel(m.CommandProgress)
	if started.Percent != 16 || started.CommandCurrent != 2 || started.CommandTotal != 3 {
		t.Fatalf("START view = %+v, want 16%% at 2/3", started)
	}

	m, _ = updateCommand(m, commandEvent("agent:pi", 2, pipeline.CommandProgressSucceeded))
	succeeded := m.Progress.ViewModel(m.CommandProgress)
	if succeeded.Percent != 33 {
		t.Fatalf("SUCCEEDED percent = %d, want 33", succeeded.Percent)
	}

	m, _ = updateCommand(m, commandEvent("agent:pi", 2, pipeline.CommandProgressStarted))
	delayed := m.Progress.ViewModel(m.CommandProgress)
	if delayed.Percent != succeeded.Percent {
		t.Fatalf("delayed START percent = %d, want %d", delayed.Percent, succeeded.Percent)
	}
	m, _ = updateCommand(m, commandEvent("agent:pi", 1, pipeline.CommandProgressStarted))
	stale := m.Progress.ViewModel(m.CommandProgress)
	if stale.Percent != succeeded.Percent || stale.CommandCurrent != 2 {
		t.Fatalf("stale replay view = %+v, want monotonic 2/3 at %d%%", stale, succeeded.Percent)
	}

	m, _ = updateCommand(m, commandEvent("agent:pi", 3, pipeline.CommandProgressFailed))
	failed := m.Progress.ViewModel(m.CommandProgress)
	if failed.Percent != succeeded.Percent || failed.CommandCurrent != 3 || failed.CommandTotal != 3 {
		t.Fatalf("FAILED view = %+v, want frozen %d%% at 3/3", failed, succeeded.Percent)
	}
}

func TestInstallViewProgressRendersAndClearsLiveSubProgress(t *testing.T) {
	m := installProgressModel().startCommandProgress()
	m, _ = updateCommand(m, commandEvent("agent:pi", 2, pipeline.CommandProgressStarted))
	if rendered := m.View(); !strings.Contains(rendered, "[2/3] Install Pi") {
		t.Fatalf("rendered install row = %q, want safe counter and label", rendered)
	}

	updated, _ := m.Update(PipelineDoneMsg{})
	if rendered := updated.(Model).View(); strings.Contains(rendered, "[2/3]") || strings.Contains(rendered, "Install Pi") {
		t.Fatalf("post-install view retained sub-progress: %q", rendered)
	}
}

func TestInstallViewProgressBypassesSingleCommandState(t *testing.T) {
	m := installProgressModel()
	withoutProgress := m.Progress.ViewModel()
	m.CommandProgress = CommandProgressState{StepID: "agent:pi", Current: 1, Total: 1, Completed: 1, DisplayName: "Install Pi", LastStatus: pipeline.CommandProgressSucceeded}
	withSingleCommand := m.Progress.ViewModel(m.CommandProgress)
	if withSingleCommand.Percent != withoutProgress.Percent || withSingleCommand.CommandTotal != 0 || withSingleCommand.CommandDisplayName != "" {
		t.Fatalf("single-command view = %+v, want compatibility with %+v", withSingleCommand, withoutProgress)
	}
}

func TestPipelineDoneUsesTerminalResultWithoutPersistingCommandProgress(t *testing.T) {
	m := installProgressModel().startCommandProgress()
	m, _ = updateCommand(m, commandEvent("agent:pi", 2, pipeline.CommandProgressStarted))

	result := pipeline.ExecutionResult{
		Apply: pipeline.StageResult{
			Success: true,
			Steps: []pipeline.StepResult{
				{StepID: "agent:pi", Status: pipeline.StepStatusSucceeded},
				{StepID: "next", Status: pipeline.StepStatusSucceeded},
			},
		},
	}
	updated, _ := m.Update(PipelineDoneMsg{Result: result})
	state := updated.(Model)
	rendered := state.View()

	if len(state.Execution.Apply.Steps) != 2 || state.Execution.Apply.Steps[0].StepID != "agent:pi" || state.Execution.Apply.Steps[0].Status != pipeline.StepStatusSucceeded {
		t.Fatalf("terminal execution result = %+v, want completed pipeline steps", state.Execution)
	}
	if state.CommandProgress != (CommandProgressState{}) {
		t.Fatalf("terminal result retained ephemeral command progress: %+v", state.CommandProgress)
	}
	if strings.Contains(rendered, "[2/3]") || strings.Contains(rendered, "Install Pi") {
		t.Fatalf("post-install history rendered command progress: %q", rendered)
	}
}

func TestFailedSingleCommandInstallRendersFailedStepWithoutCounter(t *testing.T) {
	m := installProgressModel().startCommandProgress()
	single := pipeline.CommandProgressEvent{
		StepID:      "agent:pi",
		Current:     1,
		Total:       1,
		DisplayName: "Install Pi",
		Status:      pipeline.CommandProgressFailed,
	}
	m, _ = updateCommand(m, single)

	result := pipeline.ExecutionResult{
		Apply: pipeline.StageResult{
			Success: false,
			Steps: []pipeline.StepResult{
				{StepID: "agent:pi", Status: pipeline.StepStatusFailed},
				{StepID: "next", Status: pipeline.StepStatusSucceeded},
			},
		},
	}
	updated, _ := m.Update(PipelineDoneMsg{Result: result})
	state := updated.(Model)
	rendered := state.View()

	if !state.Progress.HasFailures() || state.Progress.Items[0].Status != string(pipeline.StepStatusFailed) {
		t.Fatalf("failed single-command step = %+v, want visible failed state", state.Progress)
	}
	if !strings.Contains(rendered, "agent:pi") || !strings.Contains(rendered, "Completed with errors: 1 succeeded, 1 failed") {
		t.Fatalf("failed single-command install did not render normal failure state: %q", rendered)
	}
	if strings.Contains(rendered, "[1/1]") || strings.Contains(rendered, "Install Pi") {
		t.Fatalf("failed single-command install rendered command counter or label: %q", rendered)
	}
}
