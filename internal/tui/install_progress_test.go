package tui

import (
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
	updated, cmd := m.Update(commandProgressMsg{event: event, generation: m.commandProgressGeneration})
	return updated.(Model), cmd
}

func TestCommandProgressTransportDoesNotBlockAndResubscribes(t *testing.T) {
	m := installProgressModel()
	m = m.startCommandProgress()
	finished := make(chan struct{})
	go func() {
		for i := 0; i < commandProgressBuffer+1; i++ {
			m.commandProgress(commandEvent("agent:pi", 1, pipeline.CommandProgressStarted))
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
	m.commandProgressEvents = make(chan pipeline.CommandProgressEvent, 2)
	m.commandProgress(commandEvent("agent:pi", 1, pipeline.CommandProgressStarted))
	m.commandProgress(commandEvent("agent:pi", 2, pipeline.CommandProgressSucceeded))
	cmd := m.listenForCommandProgress()
	first := cmd().(commandProgressMsg)
	m, cmd = updateCommand(m, first.event)
	if m.CommandProgress.Current != 1 || cmd == nil {
		t.Fatalf("first event = %+v, listener = %v", m.CommandProgress, cmd != nil)
	}
	second := cmd().(commandProgressMsg)
	m, _ = updateCommand(m, second.event)
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

	stale := commandProgressMsg{event: commandEvent("agent:pi", 2, pipeline.CommandProgressStarted), generation: m.commandProgressGeneration - 1}
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
