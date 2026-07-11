package cli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/lumberbarons/issues/internal/conventions"
	"github.com/lumberbarons/issues/internal/gh"
	"github.com/lumberbarons/issues/internal/model"
	"github.com/lumberbarons/issues/internal/render"
)

// CreateOpts are the create command's inputs.
type CreateOpts struct {
	Title          string
	Type           string
	Priority       string // empty means DefaultPriority
	Areas          []string
	BlockedBy      []int
	Parent         int
	DiscoveredFrom int
	BodyFile       string
	Edit           bool
}

// Create files a new issue conforming to the conventions: exactly one
// priority and one type label, template-scaffolded body, native
// dependencies and parent links.
func (a *App) Create(ctx context.Context, opts CreateOpts) error {
	if opts.Title == "" {
		return usageErr("--title is required")
	}
	if !model.IsType(opts.Type) {
		return usageErr("--type must be one of %s", strings.Join(model.Types, "|"))
	}
	priority := model.DefaultPriority
	if opts.Priority != "" {
		p, ok := model.ParsePriority(opts.Priority)
		if !ok {
			return usageErr("--priority must be P0..P4")
		}
		priority = p
	}
	if opts.BodyFile != "" && opts.Edit {
		return usageErr("--body-file and --edit are mutually exclusive")
	}

	body, err := a.composeBody(opts)
	if err != nil {
		return err
	}

	labels := append([]string{priority.String(), opts.Type}, opts.Areas...)
	created, err := a.Client.CreateIssue(ctx, opts.Title, body, labels)
	if err != nil {
		return err
	}
	// A brand-new issue has no dependents, so --blocked-by can't create a
	// cycle; no transitive check needed on this path.
	for _, blocker := range opts.BlockedBy {
		blockerIssue, err := a.Client.GetIssue(ctx, blocker)
		if err != nil {
			return fmt.Errorf("created #%d but --blocked-by %d failed: %w", created.Number, blocker, err)
		}
		if err := a.Client.AddBlockedBy(ctx, created.ID, blockerIssue.ID); err != nil {
			return fmt.Errorf("created #%d but --blocked-by %d failed: %w", created.Number, blocker, err)
		}
	}
	if opts.Parent > 0 {
		parent, err := a.Client.GetIssue(ctx, opts.Parent)
		if err != nil {
			return fmt.Errorf("created #%d but --parent %d failed: %w", created.Number, opts.Parent, err)
		}
		if err := a.Client.AddSubIssue(ctx, parent.ID, created.ID, false); err != nil {
			return fmt.Errorf("created #%d but --parent %d failed: %w", created.Number, opts.Parent, err)
		}
	}
	return a.reportMutation(ctx, created.Number, "created #%d: %s\n", created.Number, opts.Title)
}

func (a *App) composeBody(opts CreateOpts) (string, error) {
	body := ""
	switch {
	case opts.BodyFile != "":
		b, err := os.ReadFile(opts.BodyFile)
		if err != nil {
			return "", genericErr("cannot read --body-file: %v", err)
		}
		body = string(b)
	case opts.Edit:
		if a.Edit == nil {
			return "", genericErr("--edit is not available here")
		}
		edited, err := a.Edit(conventions.TemplateSkeleton(opts.Type))
		if err != nil {
			return "", genericErr("editor failed: %v", err)
		}
		body = conventions.StripEmptySections(edited)
	}
	if opts.DiscoveredFrom > 0 {
		link := conventions.DiscoveredFrom(opts.DiscoveredFrom)
		if body == "" {
			body = link
		} else {
			body = strings.TrimRight(body, "\n") + "\n\n" + link
		}
	}
	return body, nil
}

// Start claims an issue: assign @me plus the in-progress label. The guard
// refuses issues that are already assigned or in-progress (exit 3) so an
// agent loop moves on to the next ready item; --force steals. Claiming an
// untriaged issue requires --priority — claiming forces triage.
func (a *App) Start(ctx context.Context, number int, priorityFlag string, force bool) error {
	issue, err := a.Client.GetIssue(ctx, number)
	if err != nil {
		return err
	}
	if !issue.IsOpen() {
		return genericErr("#%d is closed", number)
	}
	if issue.IsEpic() {
		return genericErr("#%d is an epic; epics are never worked directly — start one of its sub-issues", number)
	}
	if (len(issue.Assignees) > 0 || issue.InProgress()) && !force {
		who := "in-progress"
		if len(issue.Assignees) > 0 {
			who = "assigned to @" + strings.Join(issue.Assignees, " @")
		}
		return &ExitError{Code: ExitClaimed, Message: fmt.Sprintf("#%d already claimed (%s); pick the next ready item or --force", number, who)}
	}
	priority, _ := issue.Priority()
	if priorityFlag != "" {
		p, ok := model.ParsePriority(priorityFlag)
		if !ok {
			return usageErr("--priority must be P0..P4")
		}
		priority = p
	} else if priority == model.PriorityUnknown {
		return usageErr("#%d is untriaged; start requires --priority (claiming is triage)", number)
	}

	viewer, err := a.Client.Viewer(ctx)
	if err != nil {
		return err
	}
	if err := a.swapPriority(ctx, issue, priority); err != nil {
		return err
	}
	if force && len(issue.Assignees) > 0 {
		if err := a.Client.RemoveAssignees(ctx, number, issue.Assignees); err != nil {
			return err
		}
	}
	if !issue.InProgress() {
		if err := a.Client.AddLabels(ctx, number, []string{model.InProgressLabel}); err != nil {
			return err
		}
	}
	if err := a.Client.AddAssignee(ctx, number, viewer); err != nil {
		return err
	}
	// GitHub has no conditional writes, so the guard is check-then-act:
	// re-read and make sure we're the only claimant.
	after, err := a.Client.GetIssue(ctx, number)
	if err != nil {
		return err
	}
	if len(after.Assignees) != 1 || after.Assignees[0] != viewer {
		a.warnf("claim may have raced: #%d now assigned to @%s", number, strings.Join(after.Assignees, " @"))
	}
	if a.JSON {
		return render.JSONIssue(a.Out, after)
	}
	a.printf("started #%d: %s\n", number, issue.Title)
	return nil
}

// SetOpts are the retriage/edit inputs; zero values mean "leave alone".
type SetOpts struct {
	Priority    string
	Type        string
	AddAreas    []string
	RemoveAreas []string
	Parent      int
	NoParent    bool
	Title       string
}

func (o SetOpts) empty() bool {
	return o.Priority == "" && o.Type == "" && len(o.AddAreas) == 0 &&
		len(o.RemoveAreas) == 0 && o.Parent == 0 && !o.NoParent && o.Title == ""
}

// Set retriages or edits within the conventions, swapping the old
// priority/type label rather than stacking a second one.
func (a *App) Set(ctx context.Context, number int, opts SetOpts) error {
	if opts.empty() {
		return usageErr("nothing to change; pass --priority, --type, --add-area, --remove-area, --parent, --no-parent, or --title")
	}
	if opts.Parent > 0 && opts.NoParent {
		return usageErr("--parent and --no-parent are mutually exclusive")
	}
	issue, err := a.Client.GetIssue(ctx, number)
	if err != nil {
		return err
	}
	if opts.Priority != "" {
		p, ok := model.ParsePriority(opts.Priority)
		if !ok {
			return usageErr("--priority must be P0..P4")
		}
		if err := a.swapPriority(ctx, issue, p); err != nil {
			return err
		}
	}
	if opts.Type != "" {
		if !model.IsType(opts.Type) {
			return usageErr("--type must be one of %s", strings.Join(model.Types, "|"))
		}
		for _, l := range issue.Labels {
			if model.IsType(l) && l != opts.Type {
				if err := a.Client.RemoveLabel(ctx, number, l); err != nil {
					return err
				}
			}
		}
		if !slices.Contains(issue.Labels, opts.Type) {
			if err := a.Client.AddLabels(ctx, number, []string{opts.Type}); err != nil {
				return err
			}
		}
	}
	if len(opts.AddAreas) > 0 {
		if err := a.Client.AddLabels(ctx, number, opts.AddAreas); err != nil {
			return err
		}
	}
	for _, area := range opts.RemoveAreas {
		if err := a.Client.RemoveLabel(ctx, number, area); err != nil {
			return err
		}
	}
	if opts.Title != "" {
		if err := a.Client.EditTitle(ctx, number, opts.Title); err != nil {
			return err
		}
	}
	if opts.Parent > 0 {
		parent, err := a.Client.GetIssue(ctx, opts.Parent)
		if err != nil {
			return err
		}
		// replaceParent moves the issue when it already has one.
		if err := a.Client.AddSubIssue(ctx, parent.ID, issue.ID, true); err != nil {
			return err
		}
	}
	if opts.NoParent {
		if issue.Parent == nil {
			a.warnf("#%d has no parent; --no-parent is a no-op", number)
		} else {
			parent, err := a.Client.GetIssue(ctx, issue.Parent.Number)
			if err != nil {
				return err
			}
			if err := a.Client.RemoveSubIssue(ctx, parent.ID, issue.ID); err != nil {
				return err
			}
		}
	}
	return a.reportMutation(ctx, number, "updated #%d\n", number)
}

// Close comments the reason and closes: not-planned unless --completed or
// --duplicate-of. Closing via PR is the norm; this is for wontfix/duplicate.
func (a *App) Close(ctx context.Context, number int, reason string, completed bool, duplicateOf int) error {
	if completed && duplicateOf > 0 {
		return usageErr("--completed and --duplicate-of are mutually exclusive")
	}
	stateReason := "NOT_PLANNED"
	switch {
	case completed:
		stateReason = "COMPLETED"
	case duplicateOf > 0:
		stateReason = "DUPLICATE"
		if reason == "" {
			reason = fmt.Sprintf("Duplicate of #%d", duplicateOf)
		}
	}
	if reason == "" {
		return usageErr("--reason is required")
	}
	issue, err := a.Client.GetIssue(ctx, number)
	if err != nil {
		return err
	}
	if !issue.IsOpen() {
		return genericErr("#%d is already closed", number)
	}
	if err := a.Client.Comment(ctx, number, reason); err != nil {
		return err
	}
	if err := a.Client.CloseIssue(ctx, issue.ID, stateReason); err != nil {
		return err
	}
	if a.JSON {
		return a.reportMutation(ctx, number, "")
	}
	a.printf("closed #%d (%s)\n", number, strings.ToLower(strings.ReplaceAll(stateReason, "_", " ")))
	return nil
}

// Block adds a native dependency after a transitive client-side cycle
// check — GitHub itself only rejects self-blocks and direct two-issue
// cycles.
func (a *App) Block(ctx context.Context, number, blocker int) error {
	issues, err := a.Client.ListIssues(ctx, openStates)
	if err != nil {
		return err
	}
	byNum := model.ByNumber(issues)
	issue, ok := byNum[number]
	if !ok {
		return genericErr("#%d is not an open issue in %s", number, a.Repo)
	}
	blockerIssue, ok := byNum[blocker]
	if !ok {
		return genericErr("#%d is not an open issue in %s; closed blockers don't block", blocker, a.Repo)
	}
	if slices.Contains(issue.OpenBlockers(), blocker) {
		a.printf("#%d is already blocked by #%d\n", number, blocker)
		return nil
	}
	check := model.CheckBlockedBy(issues, number, blocker)
	if check.Cycle != nil {
		return genericErr("refusing: would create dependency cycle %s", render.FormatCycle(check.Cycle))
	}
	if !check.Verifiable {
		return genericErr("refusing: cannot verify #%d → #%d is cycle-free because some issues have more blockers than were fetched; reduce blockers on the issues involved and retry", number, blocker)
	}
	if err := a.Client.AddBlockedBy(ctx, issue.ID, blockerIssue.ID); err != nil {
		return err
	}
	return a.reportMutation(ctx, number, "blocked #%d on #%d\n", number, blocker)
}

// Unblock removes a dependency.
func (a *App) Unblock(ctx context.Context, number, blocker int) error {
	issue, err := a.Client.GetIssue(ctx, number)
	if err != nil {
		return err
	}
	found := false
	for _, b := range issue.BlockedBy {
		if b.Number == blocker {
			found = true
		}
	}
	if !found {
		a.printf("#%d is not blocked by #%d\n", number, blocker)
		return nil
	}
	blockerIssue, err := a.Client.GetIssue(ctx, blocker)
	if err != nil {
		return err
	}
	if err := a.Client.RemoveBlockedBy(ctx, issue.ID, blockerIssue.ID); err != nil {
		return err
	}
	return a.reportMutation(ctx, number, "unblocked #%d from #%d\n", number, blocker)
}

// EpicCreate files a parent issue and attaches existing children. Epics
// get the cosmetic title prefix and a priority label but no type — they
// are containers, not work.
func (a *App) EpicCreate(ctx context.Context, title string, children []int) error {
	if title == "" {
		return usageErr("--title is required")
	}
	if !strings.HasPrefix(title, conventions.EpicTitlePrefix) {
		title = conventions.EpicTitlePrefix + title
	}
	created, err := a.Client.CreateIssue(ctx, title, "", []string{model.DefaultPriority.String()})
	if err != nil {
		return err
	}
	for _, child := range children {
		childIssue, err := a.Client.GetIssue(ctx, child)
		if err != nil {
			return fmt.Errorf("created epic #%d but attaching #%d failed: %w", created.Number, child, err)
		}
		if err := a.Client.AddSubIssue(ctx, created.ID, childIssue.ID, false); err != nil {
			return fmt.Errorf("created epic #%d but attaching #%d failed: %w", created.Number, child, err)
		}
	}
	return a.reportMutation(ctx, created.Number, "created epic #%d: %s (%d children)\n", created.Number, title, len(children))
}

// Init bootstraps the convention labels in the repo and prints the
// CLAUDE.md snippet.
func (a *App) Init(ctx context.Context) error {
	existing, err := a.Client.ListLabels(ctx)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, l := range existing {
		have[l.Name] = true
	}
	var created []string
	for _, l := range conventions.Labels {
		if have[l.Name] {
			continue
		}
		if err := a.Client.CreateLabel(ctx, gh.Label{Name: l.Name, Color: l.Color, Description: l.Description}); err != nil {
			return fmt.Errorf("creating label %q: %w", l.Name, err)
		}
		created = append(created, l.Name)
	}
	if a.JSON {
		if created == nil {
			created = []string{}
		}
		return render.WriteJSON(a.Out, map[string]any{"createdLabels": created})
	}
	if len(created) == 0 {
		a.printf("all convention labels already exist in %s\n", a.Repo)
	} else {
		a.printf("created labels: %s\n", strings.Join(created, ", "))
	}
	a.printf("\nAdd to CLAUDE.md:\n\n%s\n", conventions.ClaudeSnippet)
	a.printf("\nOr let a hook inject the primer automatically: issues hooks install\n")
	return nil
}

// swapPriority enforces the one-priority-label invariant: remove the
// others, add the target if absent.
func (a *App) swapPriority(ctx context.Context, issue model.Issue, p model.Priority) error {
	target := p.String()
	for _, l := range issue.Labels {
		if _, ok := model.ParsePriority(l); ok && l != target {
			if err := a.Client.RemoveLabel(ctx, issue.Number, l); err != nil {
				return err
			}
		}
	}
	if !slices.Contains(issue.Labels, target) {
		return a.Client.AddLabels(ctx, issue.Number, []string{target})
	}
	return nil
}

// reportMutation prints the text confirmation, or re-fetches for the full
// flat schema when --json is on.
func (a *App) reportMutation(ctx context.Context, number int, format string, args ...any) error {
	if a.JSON {
		after, err := a.Client.GetIssue(ctx, number)
		if err != nil {
			return err
		}
		return render.JSONIssue(a.Out, after)
	}
	if format != "" {
		a.printf(format, args...)
	}
	return nil
}
