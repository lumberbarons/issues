// Package cli implements the commands against the gh.Client interface, so
// cmd/issues is pure wiring and everything with behavior is testable
// against a fake.
package cli

import (
	"fmt"
	"io"

	"github.com/lumberbarons/issues/internal/gh"
)

// Exit codes, kept meaningful so agent loops can branch on them.
const (
	ExitGeneric = 1
	ExitUsage   = 2
	// ExitClaimed is start's "already claimed" refusal: pick the next
	// ready item instead of doubling up.
	ExitClaimed = 3
	ExitAuth    = 4
)

// ExitError carries a specific exit code to main.
type ExitError struct {
	Code    int
	Message string
}

func (e *ExitError) Error() string { return e.Message }

func usageErr(format string, args ...any) *ExitError {
	return &ExitError{Code: ExitUsage, Message: fmt.Sprintf(format, args...)}
}

func genericErr(format string, args ...any) *ExitError {
	return &ExitError{Code: ExitGeneric, Message: fmt.Sprintf(format, args...)}
}

// App holds the dependencies every command needs.
type App struct {
	Client gh.Client
	Repo   gh.Repo
	Out    io.Writer
	ErrOut io.Writer
	JSON   bool
	// Edit opens an editor seeded with initial text and returns the result;
	// wired to $EDITOR by main, injected by tests.
	Edit func(initial string) (string, error)
}

func (a *App) printf(format string, args ...any) {
	fmt.Fprintf(a.Out, format, args...)
}

func (a *App) warnf(format string, args ...any) {
	fmt.Fprintf(a.ErrOut, "⚠ "+format+"\n", args...)
}

// progressf narrates long-running work: stdout normally, stderr under
// --json so the machine-readable stream stays clean.
func (a *App) progressf(format string, args ...any) {
	if a.JSON {
		fmt.Fprintf(a.ErrOut, format, args...)
		return
	}
	fmt.Fprintf(a.Out, format, args...)
}
