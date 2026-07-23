package upgrade

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
)

// This file holds RED tests for the target-plugin exact verifier, the one-retry
// contract, the process-safety seams, and the scope boundary (PR 1 slice of
// fix-opencode-plugin-upgrade-verification). The production helpers
// (runStrategyWithOutcome, strategyOutcome, the exact manifest inspector, and
// the retry contract inside opencodePluginUpgrade) do not exist yet — these
// tests reference them and therefore MUST fail until GREEN.

// targetPluginResult builds an UpdateResult for the in-scope target plugin.
func targetPluginResult(latestVersion string) update.UpdateResult {
	pkg := "opencode-subagent-statusline"
	return update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          pkg,
			InstallMethod: update.InstallOpenCodePlugin,
			NpmPackage:    pkg,
		},
		LatestVersion: latestVersion,
		Status:        update.UpdateAvailable,
	}
}

// materializeTarget writes the target plugin's package.json manifest with the
// given version under the OpenCode directory.
func materializeTarget(t *testing.T, opencodeDir, version string) {
	t.Helper()
	pkgDir := filepath.Join(opencodeDir, "node_modules", "opencode-subagent-statusline")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"version":"`+version+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// registerTargetInTUI lists the target plugin in tui.json so the upgrade
// precheck (openCodePluginRegisteredOrMaterialized) passes and the package-
// manager command is actually issued, even when the manifest is absent.
func registerTargetInTUI(t *testing.T, opencodeDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(opencodeDir, "tui.json"), []byte(`{"plugin":["opencode-subagent-statusline"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// openCodeVerifierTest wires the OpenCode home/lookpath seams to a temp home,
// selects bun, and routes execCommand through the supplied command factory.
// It returns the OpenCode dir so the test can pre-materialize manifests.
func openCodeVerifierTest(t *testing.T, command func(string, ...string) *exec.Cmd) (home, opencodeDir string) {
	t.Helper()
	origHomeDir, origLookPath, origExecCommand := openCodeHomeDir, lookPathCommand, execCommand
	t.Cleanup(func() {
		openCodeHomeDir, lookPathCommand, execCommand = origHomeDir, origLookPath, origExecCommand
	})
	home = t.TempDir()
	opencodeDir = filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	openCodeHomeDir = func() (string, error) { return home, nil }
	lookPathCommand = func(file string) (string, error) {
		if file == "bun" {
			return "/usr/bin/bun", nil
		}
		return "", errors.New("not found")
	}
	execCommand = command
	return home, opencodeDir
}

// successCmd returns an exec.Cmd whose helper process exits 0 and writes its
// cwd to the given file (so tests can assert cmd.Dir). Each invocation creates
// a fresh cwd file so retries are distinguishable.
func successCmd(t *testing.T, cwdFiles *[]string) func(string, ...string) *exec.Cmd {
	t.Helper()
	return func(name string, args ...string) *exec.Cmd {
		cwdFile := filepath.Join(t.TempDir(), "cwd.txt")
		*cwdFiles = append(*cwdFiles, cwdFile)
		cmd := exec.Command(os.Args[0], "-test.run=TestOpenCodePluginUpgradeHelperProcess", "--")
		cmd.Env = append(os.Environ(),
			"GENTLE_AI_UPGRADE_HELPER=1",
			"GENTLE_AI_UPGRADE_HELPER_CWD_FILE="+cwdFile,
		)
		return cmd
	}
}

// asManualFallbackHint unwraps err into a ManualFallbackError hint, failing the
// test when err is nil or not a manual fallback.
func asManualFallbackHint(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected ManualFallbackError, got nil")
	}
	hint, ok := AsManualFallback(err)
	if !ok {
		t.Fatalf("expected ManualFallbackError, got %T: %v", err, err)
	}
	return hint
}

func TestOpenCodePluginVerifier_ExactVersionMaterializesSuccess(t *testing.T) {
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	materializeTarget(t, opencodeDir, "0.8.0")

	outcome, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{PackageManager: "brew"})
	if err != nil {
		t.Fatalf("runStrategyWithOutcome: unexpected error: %v", err)
	}
	if outcome.exitRequested {
		t.Fatalf("exitRequested = true, want false for OpenCode plugin")
	}
	if outcome.observedVersion != "0.8.0" {
		t.Fatalf("observedVersion = %q, want %q", outcome.observedVersion, "0.8.0")
	}
}

func TestOpenCodePluginVerifier_StaleVersionAfterSuccessIsNotSuccess(t *testing.T) {
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	// Materialize the OLD version; the verifier must observe it and reject.
	materializeTarget(t, opencodeDir, "0.7.1")

	_, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	hint := asManualFallbackHint(t, err)
	for _, want := range []string{"0.8.0", "0.7.1"} {
		if !strings.Contains(hint, want) {
			t.Errorf("manual hint %q does not contain expected version %q", hint, want)
		}
	}
}

func TestOpenCodePluginVerifier_OlderObservedMismatch(t *testing.T) {
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	materializeTarget(t, opencodeDir, "0.7.2")

	_, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	hint := asManualFallbackHint(t, err)
	if !strings.Contains(hint, "0.8.0") || !strings.Contains(hint, "0.7.2") {
		t.Fatalf("manual hint %q should reference expected 0.8.0 and observed 0.7.2", hint)
	}
}

func TestOpenCodePluginVerifier_NewerObservedMismatch(t *testing.T) {
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	materializeTarget(t, opencodeDir, "0.9.0")

	_, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	hint := asManualFallbackHint(t, err)
	if !strings.Contains(hint, "0.9.0") {
		t.Fatalf("manual hint %q should flag the unexpected observed 0.9.0", hint)
	}
}

func TestOpenCodePluginVerifier_AbsentManifestAfterSuccessIsNotSuccess(t *testing.T) {
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	registerTargetInTUI(t, opencodeDir)
	// No node_modules manifest at all -> absent. One retry, then manual fallback.
	_, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	hint := asManualFallbackHint(t, err)
	if !strings.Contains(hint, "absent") && !strings.Contains(hint, "0.8.0") {
		t.Fatalf("manual hint %q should mention absent and expected 0.8.0", hint)
	}
	if len(cwdFiles) != 2 {
		t.Fatalf("exec calls = %d, want exactly 2 (initial + one retry) for absent manifest", len(cwdFiles))
	}
}

func TestOpenCodePluginVerifier_MalformedManifestAfterSuccessIsNotSuccess(t *testing.T) {
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	pkgDir := filepath.Join(opencodeDir, "node_modules", "opencode-subagent-statusline")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{not valid json`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	hint := asManualFallbackHint(t, err)
	if !strings.Contains(hint, "0.8.0") {
		t.Fatalf("manual hint %q should reference expected 0.8.0", hint)
	}
	if len(cwdFiles) != 2 {
		t.Fatalf("exec calls = %d, want exactly 2 (initial + one retry) for malformed manifest", len(cwdFiles))
	}
}

func TestOpenCodePluginVerifier_UnreadableManifestAfterSuccessIsNotSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not honored on Windows")
	}
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	pkgDir := filepath.Join(opencodeDir, "node_modules", "opencode-subagent-statusline")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"version":"0.8.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(pkgDir, "package.json"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(pkgDir, "package.json"), 0o644) })

	_, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	hint := asManualFallbackHint(t, err)
	if !strings.Contains(hint, "0.8.0") {
		t.Fatalf("manual hint %q should reference expected 0.8.0", hint)
	}
	if len(cwdFiles) != 2 {
		t.Fatalf("exec calls = %d, want exactly 2 (initial + one retry) for unreadable manifest", len(cwdFiles))
	}
}

func TestOpenCodePluginVerifier_RetryRestoresMaterialization(t *testing.T) {
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	registerTargetInTUI(t, opencodeDir)
	// First attempt: absent manifest. Second attempt: materialize exactly.
	var callCount int
	wrapped := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		callCount++
		// Materialize the target when the SECOND command is constructed. By then
		// the first attempt's inspection has already seen "absent", and the
		// second command (and its subsequent inspection) will observe 0.8.0.
		if callCount == 2 {
			materializeTarget(t, opencodeDir, "0.8.0")
		}
		return wrapped(name, args...)
	}
	t.Cleanup(func() { execCommand = wrapped })

	outcome, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	if err != nil {
		t.Fatalf("retry should restore success, got error: %v", err)
	}
	if outcome.observedVersion != "0.8.0" {
		t.Fatalf("observedVersion = %q, want 0.8.0 after retry", outcome.observedVersion)
	}
	if callCount != 2 {
		t.Fatalf("exec calls = %d, want exactly 2 (initial + one retry)", callCount)
	}
}

func TestOpenCodePluginVerifier_PersistentFailureRetriesAtMostOnce(t *testing.T) {
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	registerTargetInTUI(t, opencodeDir)
	_, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	asManualFallbackHint(t, err)
	if len(cwdFiles) != 2 {
		t.Fatalf("exec calls = %d, want exactly 2; the verifier must retry at most once", len(cwdFiles))
	}
}

func TestOpenCodePluginVerifier_CancelledContextDoesNotRetry(t *testing.T) {
	var cwdFiles []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, opencodeDir := openCodeVerifierTest(t, func(name string, args ...string) *exec.Cmd {
		cancel() // cancel before the retry could start
		cwdFile := filepath.Join(t.TempDir(), "cwd.txt")
		cwdFiles = append(cwdFiles, cwdFile)
		cmd := exec.Command(os.Args[0], "-test.run=TestOpenCodePluginUpgradeHelperProcess", "--")
		cmd.Env = append(os.Environ(),
			"GENTLE_AI_UPGRADE_HELPER=1",
			"GENTLE_AI_UPGRADE_HELPER_CWD_FILE="+cwdFile,
		)
		return cmd
	})
	registerTargetInTUI(t, opencodeDir)

	_, err := runStrategyWithOutcome(ctx, targetPluginResult("0.8.0"), system.PlatformProfile{})
	if err == nil {
		t.Fatal("expected cancellation to prevent retry and surface an error, got nil")
	}
	if len(cwdFiles) != 1 {
		t.Fatalf("exec calls = %d, want 1 (context cancelled before retry)", len(cwdFiles))
	}
}

func TestOpenCodePluginVerifier_FixedArgv(t *testing.T) {
	var cwdFiles []string
	var observed [][]string
	_, opencodeDir := openCodeVerifierTest(t, func(name string, args ...string) *exec.Cmd {
		observed = append(observed, append([]string{name}, args...))
		cwdFile := filepath.Join(t.TempDir(), "cwd.txt")
		cwdFiles = append(cwdFiles, cwdFile)
		cmd := exec.Command(os.Args[0], "-test.run=TestOpenCodePluginUpgradeHelperProcess", "--")
		cmd.Env = append(os.Environ(),
			"GENTLE_AI_UPGRADE_HELPER=1",
			"GENTLE_AI_UPGRADE_HELPER_CWD_FILE="+cwdFile,
		)
		return cmd
	})
	materializeTarget(t, opencodeDir, "0.8.0")

	_, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(observed) != 1 {
		t.Fatalf("exec calls = %d, want 1 on exact materialization", len(observed))
	}
	want := []string{"bun", "add", "opencode-subagent-statusline@latest", "@opencode-ai/plugin@latest"}
	if strings.Join(observed[0], " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %v, want %v", observed[0], want)
	}
}

func TestOpenCodePluginVerifier_UsesConfiguredOpenCodeDirAsCwd(t *testing.T) {
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	materializeTarget(t, opencodeDir, "0.8.0")

	_, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cwdFiles) != 1 {
		t.Fatalf("exec calls = %d, want 1", len(cwdFiles))
	}
	gotCwd, err := os.ReadFile(cwdFiles[0])
	if err != nil {
		t.Fatalf("read helper cwd: %v", err)
	}
	resolvedGot, err := filepath.EvalSymlinks(strings.TrimSpace(string(gotCwd)))
	if err != nil {
		t.Fatalf("eval got cwd: %v", err)
	}
	resolvedWant, err := filepath.EvalSymlinks(opencodeDir)
	if err != nil {
		t.Fatalf("eval want cwd: %v", err)
	}
	if resolvedGot != resolvedWant {
		t.Fatalf("cmd.Dir = %q, want %q", resolvedGot, resolvedWant)
	}
}

func TestOpenCodePluginVerifier_NilStdinAndNoninteractiveEnv(t *testing.T) {
	type capturedCmd struct {
		stdin interface{}
		env   []string
	}
	var captured []capturedCmd
	_, opencodeDir := openCodeVerifierTest(t, func(name string, args ...string) *exec.Cmd {
		// Construct the real command the production path would build, record its
		// Stdin/Env seams, then return a runnable helper so CombinedOutput works.
		realCmd := exec.Command(name, args...)
		realCmd.Stdin = nil
		realCmd.Env = openCodePluginUpgradeEnv(nil)
		captured = append(captured, capturedCmd{stdin: realCmd.Stdin, env: append([]string(nil), realCmd.Env...)})
		cwdFile := filepath.Join(t.TempDir(), "cwd.txt")
		cmd := exec.Command(os.Args[0], "-test.run=TestOpenCodePluginUpgradeHelperProcess", "--")
		cmd.Env = append(os.Environ(),
			"GENTLE_AI_UPGRADE_HELPER=1",
			"GENTLE_AI_UPGRADE_HELPER_CWD_FILE="+cwdFile,
		)
		return cmd
	})
	materializeTarget(t, opencodeDir, "0.8.0")

	_, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(captured) != 1 {
		t.Fatalf("exec calls = %d, want 1", len(captured))
	}
	if captured[0].stdin != nil {
		t.Errorf("cmd.Stdin = %v, want nil", captured[0].stdin)
	}
	for _, want := range []string{"CI=1", "npm_config_yes=true", "npm_config_audit=false", "npm_config_fund=false"} {
		if !slicesContain(captured[0].env, want) {
			t.Errorf("env missing %q; env=%v", want, captured[0].env)
		}
	}
}

func TestOpenCodePluginVerifier_ManualFallbackReferencesOpenCodeDir(t *testing.T) {
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	registerTargetInTUI(t, opencodeDir)
	_, err := runStrategyWithOutcome(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	hint := asManualFallbackHint(t, err)
	if !strings.Contains(hint, opencodeDir) {
		t.Errorf("manual hint %q should reference the configured OpenCode directory %q", hint, opencodeDir)
	}
}

func TestOpenCodePluginVerifier_OtherPluginPreservesExitCodeBehavior(t *testing.T) {
	// Scope boundary: opencode-sdd-engram-manage must keep the prior success-on-exit-code
	// behavior in PR 1 (no verifier, no observed version).
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	pkg := "opencode-sdd-engram-manage"
	if err := os.WriteFile(filepath.Join(opencodeDir, "tui.json"), []byte(`{"plugin":["`+pkg+`"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          pkg,
			InstallMethod: update.InstallOpenCodePlugin,
			NpmPackage:    pkg,
		},
		LatestVersion: "1.2.0",
		Status:        update.RegisteredNotMaterialized,
	}
	outcome, err := runStrategyWithOutcome(context.Background(), r, system.PlatformProfile{})
	if err != nil {
		t.Fatalf("other plugin should succeed on exit code, got error: %v", err)
	}
	if outcome.observedVersion != "" {
		t.Fatalf("other plugin observedVersion = %q, want empty (PR 1 scope boundary)", outcome.observedVersion)
	}
}

func TestRunStrategy_WrapsStrategyWithOutcome(t *testing.T) {
	var cwdFiles []string
	_, opencodeDir := openCodeVerifierTest(t, successCmd(t, &cwdFiles))
	materializeTarget(t, opencodeDir, "0.8.0")

	exitReq, err := runStrategy(context.Background(), targetPluginResult("0.8.0"), system.PlatformProfile{})
	if err != nil {
		t.Fatalf("runStrategy compatibility wrapper: unexpected error: %v", err)
	}
	if exitReq {
		t.Fatalf("runStrategy exitReq = true, want false for OpenCode plugin")
	}
}

// TestRunStrategy_WrapsStrategyWithOutcome is the last verifier test; the
// capture helper scaffolding below has been removed in favor of direct cmd
// seam inspection inside the execCommand factory.
