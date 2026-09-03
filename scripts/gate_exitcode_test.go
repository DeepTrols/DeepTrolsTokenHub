package scripts

// Exit-code integrity regression for the gate command contract
// (TH-P05-13 Scope B). These tests only ever execute FAKE commands
// (sh -c exit stubs and a fake `go` shim in a temp PATH) — they never
// build, vet, or test real repository source.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// runGate executes gate_command.sh with the given wrapped command and
// returns combined output plus the gate's TRUE exit code.
//
// Isolation: any GATE_LOG_FILE inherited from the surrounding environment
// is STRIPPED before the gate runs, so these fixtures can never truncate
// or rewrite an outer gate's log file (e.g. when the full suite itself is
// executed under `GATE_LOG_FILE=... gate_command.sh go test ./...`).
// Direct-mode cases pass extraEnv=nil; the log-file case supplies its own
// private GATE_LOG_FILE explicitly.
func runGate(t *testing.T, extraEnv []string, args ...string) (string, int) {
	t.Helper()
	script, err := filepath.Abs("gate_command.sh")
	if err != nil {
		t.Fatalf("resolve gate script: %v", err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("gate_command.sh missing: %v", err)
	}
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	env := make([]string, 0, len(os.Environ())+len(extraEnv))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GATE_LOG_FILE=") {
			continue // never let fixtures write into an outer gate's log
		}
		env = append(env, kv)
	}
	cmd.Env = append(env, extraEnv...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("gate exec error (not an exit status): %v", err)
	}
	return string(out), exitErr.ExitCode()
}

// fakeGoOnPath installs a fake `go` binary at the front of PATH that exits
// with $FAKE_GO_EXIT (default 1). Real source is never touched.
func fakeGoOnPath(t *testing.T, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	shim := filepath.Join(dir, "go")
	body := "#!/bin/sh\nexit \"${FAKE_GO_EXIT:-1}\"\n"
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake go shim: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GO_EXIT", strconv.Itoa(exitCode))
}

// Case A: fake command exits 0 → gate exits 0.
func TestGate_CaseA_FakeSuccess_PropagatesZero(t *testing.T) {
	out, code := runGate(t, nil, "sh", "-c", "echo gate-ok; exit 0")
	if code != 0 {
		t.Fatalf("gate exit = %d, want 0 for a successful command", code)
	}
	if !strings.Contains(out, "gate-ok") {
		t.Errorf("gate output missing command stdout: %q", out)
	}
}

// Case B: fake command exits 1 while its output displays fine (tail/cat
// succeed) → the gate must STILL exit non-zero. Covers both direct mode
// and the GATE_LOG_FILE mode where the log display itself succeeds.
func TestGate_CaseB_FakeFailure_DisplaySuccess_StillNonZero(t *testing.T) {
	out, code := runGate(t, nil, "sh", "-c", "echo some output; exit 1")
	if code == 0 {
		t.Fatalf("gate exit = 0 despite wrapped command exit 1 (exit-code swallowing)")
	}
	if code != 1 {
		t.Errorf("gate exit = %d, want exactly 1 (wrapped command's code)", code)
	}
	if !strings.Contains(out, "some output") {
		t.Errorf("gate output missing command stdout: %q", out)
	}

	logFile := filepath.Join(t.TempDir(), "gate.log")
	out, code = runGate(t, []string{"GATE_LOG_FILE=" + logFile}, "sh", "-c", "echo logged failure; exit 1")
	if code == 0 {
		t.Fatalf("GATE_LOG_FILE mode: gate exit = 0 despite wrapped command exit 1")
	}
	if code != 1 {
		t.Errorf("GATE_LOG_FILE mode: gate exit = %d, want 1", code)
	}
	// The tail display of the log succeeded — and still did not mask status.
	if !strings.Contains(out, "logged failure") {
		t.Errorf("log tail not shown after failure: %q", out)
	}
}

// Case C: `go vet` fails (fake go shim, exit 1) → gate non-zero.
func TestGate_CaseC_FakeGoVetFailure_NonZero(t *testing.T) {
	fakeGoOnPath(t, 1)
	_, code := runGate(t, nil, "go", "vet", "./...")
	if code == 0 {
		t.Fatal("gate exit = 0 despite go vet failure (exit-code swallowing)")
	}
}

// Case D: `go build` fails (fake go shim, exit 2) → gate non-zero and the
// exact upstream code is propagated.
func TestGate_CaseD_FakeGoBuildFailure_NonZero(t *testing.T) {
	fakeGoOnPath(t, 2)
	_, code := runGate(t, nil, "go", "build", "./...")
	if code == 0 {
		t.Fatal("gate exit = 0 despite go build failure (exit-code swallowing)")
	}
	if code != 2 {
		t.Errorf("gate exit = %d, want 2 (wrapped command's code)", code)
	}
}

// Usage error: no command given → exit 2, not a hang or silent success.
func TestGate_NoCommand_ExitsTwo(t *testing.T) {
	_, code := runGate(t, nil)
	if code != 2 {
		t.Fatalf("gate exit = %d, want 2 for missing command", code)
	}
}

// Isolation regression (found during TH-P05-13 full-suite runs): if the
// surrounding environment exports GATE_LOG_FILE — e.g. the full suite is
// itself running under `GATE_LOG_FILE=... gate_command.sh go test ./...`
// — the fixtures must NEVER truncate or rewrite that outer gate log. This
// test fails if runGate stops stripping the inherited variable.
func TestGate_FixtureNeverTouchesOuterLogFile(t *testing.T) {
	outer := filepath.Join(t.TempDir(), "outer.log")
	sentinel := "outer-gate-log-sentinel\n"
	if err := os.WriteFile(outer, []byte(sentinel), 0o644); err != nil {
		t.Fatalf("write outer log: %v", err)
	}
	t.Setenv("GATE_LOG_FILE", outer) // simulate inherited environment
	_, code := runGate(t, nil, "sh", "-c", "exit 0")
	if code != 0 {
		t.Fatalf("gate exit = %d, want 0", code)
	}
	content, err := os.ReadFile(outer)
	if err != nil {
		t.Fatalf("read outer log: %v", err)
	}
	if string(content) != sentinel {
		t.Fatalf("fixture corrupted the outer gate log: got %q, want untouched sentinel", content)
	}
}

// Structural contract: pipefail must be set, and the script must never
// pipe the wrapped command into a display helper (`"$@" | ...`), which is
// exactly the masking pattern that motivated this task.
func TestGate_ScriptContract_PipefailAndNoMaskingPipe(t *testing.T) {
	content, err := os.ReadFile("gate_command.sh")
	if err != nil {
		t.Fatalf("read gate script: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "set -euo pipefail") && !strings.Contains(s, "set -o pipefail") {
		t.Error("gate script must enable pipefail")
	}
	if regexp.MustCompile(`"\$@"\s*\|`).MatchString(s) {
		t.Error(`gate script pipes the wrapped command into another tool — exit codes would be masked`)
	}
}
