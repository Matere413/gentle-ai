package screens

import (
	"strings"
	"testing"
)

func TestRenderInstallingAddsSafeCounterOnlyToActiveMultiCommandRow(t *testing.T) {
	rendered := RenderInstalling(InstallProgress{
		Percent: 33,
		Items: []ProgressItem{
			{Label: "agent:pi", Status: "running"},
			{Label: "next", Status: "pending"},
		},
		CommandItemIndex:   0,
		CommandCurrent:     2,
		CommandTotal:       3,
		CommandDisplayName: "Install Pi",
	}, "⠋")
	if !strings.Contains(rendered, "[2/3] Install Pi") || strings.Contains(rendered, "[2/3] next") {
		t.Fatalf("rendered rows = %q", rendered)
	}
}
