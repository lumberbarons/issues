// Package git reads the local branch state the pr command needs. It shells
// out to git rather than linking a git library: the CLI already assumes a
// checkout with a remote (that's how the repo is detected), and the two
// facts needed here are one command each.
package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// State is the local branch state a pull request is opened from.
type State struct {
	// Branch is the current branch name, empty on a detached HEAD.
	Branch string
	// Pushed reports whether the branch has a remote-tracking counterpart.
	// GitHub can only open a pull request for a ref it can already see, so
	// this is a precondition, not a nicety.
	Pushed bool
}

// runner runs a command and returns its trimmed stdout; injected by tests.
type runner func(name string, args ...string) (string, error)

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(out)), err
}

// Current reads the state of the checkout in the working directory.
func Current() (State, error) { return current(run) }

func current(r runner) (State, error) {
	branch, err := r("git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return State{}, fmt.Errorf("cannot read the current git branch: %w", err)
	}
	if branch == "" || branch == "HEAD" {
		return State{}, fmt.Errorf("HEAD is detached; check out a branch first")
	}
	// A failure here means "no upstream", which is a state, not an error:
	// git exits non-zero for an unset upstream exactly as it does for a
	// broken repo, and the repo can't be broken — the branch just resolved.
	upstream, err := r("git", "rev-parse", "--abbrev-ref", "@{upstream}")
	return State{Branch: branch, Pushed: err == nil && upstream != ""}, nil
}
