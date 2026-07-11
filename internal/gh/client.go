// Package gh is the thin GitHub API layer: one interface the commands are
// written against, and a go-gh-backed implementation that reuses gh's
// stored credentials.
package gh

import (
	"context"
	"errors"
	"fmt"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/lumberbarons/issues/internal/model"
)

// Repo identifies the target repository.
type Repo struct {
	Owner string
	Name  string
}

func (r Repo) String() string { return r.Owner + "/" + r.Name }

// Label mirrors a repository label for init's bootstrap pass.
type Label struct {
	Name        string
	Color       string
	Description string
}

// Client is the API surface the commands use; the tests fake it.
type Client interface {
	// Viewer returns the authenticated user's login.
	Viewer(ctx context.Context) (string, error)
	// ListIssues fetches all issues in the given states ("OPEN"/"CLOSED")
	// with labels, assignees, parent, sub-issues, and blockers, paginating
	// the outer connection. Nested connections are capped (see query).
	ListIssues(ctx context.Context, states []string) ([]model.Issue, error)
	// GetIssue fetches one issue in any state, including body and recent
	// comments.
	GetIssue(ctx context.Context, number int) (model.Issue, error)
	CreateIssue(ctx context.Context, title, body string, labels []string) (model.Issue, error)
	EditTitle(ctx context.Context, number int, title string) error
	AddLabels(ctx context.Context, number int, labels []string) error
	RemoveLabel(ctx context.Context, number int, label string) error
	AddAssignee(ctx context.Context, number int, login string) error
	RemoveAssignees(ctx context.Context, number int, logins []string) error
	Comment(ctx context.Context, number int, body string) error
	// CloseIssue closes by node ID with reason "COMPLETED", "NOT_PLANNED",
	// or "DUPLICATE".
	CloseIssue(ctx context.Context, issueID, reason string) error
	// AddBlockedBy marks issueID as blocked by blockingIssueID.
	AddBlockedBy(ctx context.Context, issueID, blockingIssueID string) error
	RemoveBlockedBy(ctx context.Context, issueID, blockingIssueID string) error
	AddSubIssue(ctx context.Context, parentID, childID string, replaceParent bool) error
	RemoveSubIssue(ctx context.Context, parentID, childID string) error
	ListLabels(ctx context.Context) ([]Label, error)
	CreateLabel(ctx context.Context, label Label) error
}

// AuthError marks failures that mean "run gh auth login", mapped to a
// distinct exit code by main.
type AuthError struct{ Err error }

func (e *AuthError) Error() string {
	return fmt.Sprintf("not authenticated (run `gh auth login`): %v", e.Err)
}

func (e *AuthError) Unwrap() error { return e.Err }

// wrapErr converts 401s into AuthError so main can exit 4.
func wrapErr(err error) error {
	if err == nil {
		return nil
	}
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == 401 {
		return &AuthError{Err: err}
	}
	return err
}
