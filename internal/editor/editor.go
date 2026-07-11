// Package editor opens $EDITOR on a seeded temp file and returns the edited
// text. The command runner is injectable so the logic is testable without a
// real editor.
package editor

import (
	"errors"
	"os"
	"os/exec"
)

// runner runs a command to completion; the real one shells out, tests
// substitute a stub.
type runner func(name string, args ...string) error

func execRun(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// Edit opens $EDITOR on a temp file seeded with initial and returns its
// contents after the editor exits.
func Edit(initial string) (string, error) {
	return edit(os.Getenv("EDITOR"), execRun, initial)
}

func edit(editor string, run runner, initial string) (string, error) {
	if editor == "" {
		return "", errors.New("$EDITOR is not set")
	}
	tmp, err := os.CreateTemp("", "issues-*.md")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(initial); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	// Pass the temp path as a positional argument ($1) rather than splicing
	// it into the command line, so a temp dir containing spaces or shell
	// metacharacters can't break or reinterpret the invocation. editor is the
	// user's own $EDITOR, so it may carry its own arguments (e.g. "code -w").
	if err := run("sh", "-c", editor+` "$1"`, "sh", tmp.Name()); err != nil {
		return "", err
	}
	edited, err := os.ReadFile(tmp.Name())
	return string(edited), err
}
