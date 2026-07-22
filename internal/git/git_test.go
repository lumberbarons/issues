package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner answers each git invocation from a table keyed by the joined
// arguments, so a test states exactly what git said and nothing else.
func fakeRunner(t *testing.T, answers map[string]string, failures map[string]error) runner {
	t.Helper()
	return func(name string, args ...string) (string, error) {
		if name != "git" {
			t.Fatalf("ran %q, want git", name)
		}
		key := strings.Join(args, " ")
		if err, ok := failures[key]; ok {
			return "", err
		}
		out, ok := answers[key]
		if !ok {
			t.Fatalf("unexpected git invocation: git %s", key)
		}
		return out, nil
	}
}

func TestCurrentReportsBranchAndUpstream(t *testing.T) {
	got, err := current(fakeRunner(t, map[string]string{
		"rev-parse --abbrev-ref HEAD":        "feat/pr-command",
		"rev-parse --abbrev-ref @{upstream}": "origin/feat/pr-command",
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.Branch != "feat/pr-command" {
		t.Errorf("Branch = %q, want feat/pr-command", got.Branch)
	}
	if !got.Pushed {
		t.Error("Pushed = false, want true when the branch has an upstream")
	}
}

// An unset upstream is a state, not a failure: git exits non-zero for it,
// and Current must report Pushed=false rather than propagating an error —
// the pr command turns it into "push it first".
func TestCurrentUnsetUpstreamIsNotAnError(t *testing.T) {
	got, err := current(fakeRunner(t,
		map[string]string{"rev-parse --abbrev-ref HEAD": "feat/local-only"},
		map[string]error{"rev-parse --abbrev-ref @{upstream}": errors.New("exit status 128")},
	))
	if err != nil {
		t.Fatalf("current() errored on an unpushed branch: %v", err)
	}
	if got.Branch != "feat/local-only" || got.Pushed {
		t.Errorf("current() = %+v, want {feat/local-only false}", got)
	}
}

// git prints "HEAD" for the branch name on a detached HEAD, which is not a
// branch a PR can be opened from.
func TestCurrentRejectsDetachedHead(t *testing.T) {
	_, err := current(fakeRunner(t, map[string]string{
		"rev-parse --abbrev-ref HEAD": "HEAD",
	}, nil))
	if err == nil || !strings.Contains(err.Error(), "detached") {
		t.Fatalf("err = %v, want a detached-HEAD error", err)
	}
}

func TestCurrentRejectsEmptyBranch(t *testing.T) {
	_, err := current(fakeRunner(t, map[string]string{
		"rev-parse --abbrev-ref HEAD": "",
	}, nil))
	if err == nil {
		t.Fatal("current() succeeded outside a branch")
	}
}

func TestCurrentReportsUnreadableRepo(t *testing.T) {
	_, err := current(fakeRunner(t, nil, map[string]error{
		"rev-parse --abbrev-ref HEAD": errors.New("not a git repository"),
	}))
	if err == nil || !strings.Contains(err.Error(), "cannot read the current git branch") {
		t.Fatalf("err = %v, want a branch-read failure", err)
	}
}

// TestCurrentAgainstRealGit drives the exported Current — the real exec
// wiring — against a throwaway repository, so a change to the argument
// strings that the fake runner would happily accept still fails here.
func TestCurrentAgainstRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--initial-branch=feat/real"},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "--allow-empty", "-m", "seed"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v failed: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "marker"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	got, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	if got.Branch != "feat/real" {
		t.Errorf("Branch = %q, want feat/real", got.Branch)
	}
	if got.Pushed {
		t.Error("Pushed = true for a repo with no remote")
	}
}
