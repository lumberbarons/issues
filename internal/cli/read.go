package cli

import (
	"context"
	"slices"

	"github.com/lumberbarons/issues/internal/conventions"
	"github.com/lumberbarons/issues/internal/model"
	"github.com/lumberbarons/issues/internal/render"
)

// primeReadyCap keeps the primer's live half inside its token budget; the
// full list is one `issues ready` away.
const primeReadyCap = 10

var (
	openStates = []string{"OPEN"}
	allStates  = []string{"OPEN", "CLOSED"}
)

// Ready lists open, non-epic, unclaimed issues with zero open blockers.
// No results is exit 0: an empty queue is an answer, not an error.
func (a *App) Ready(ctx context.Context) error {
	issues, err := a.Client.ListIssues(ctx, openStates)
	if err != nil {
		return err
	}
	for _, w := range model.WarningsOfKind(model.Warnings(issues), model.WarnDependencyCycle) {
		a.warnf("%s", render.FormatWarning(w))
	}
	ready := model.Ready(issues)
	if a.JSON {
		return render.JSONList(a.Out, ready)
	}
	if len(ready) == 0 {
		a.printf("no ready work\n")
		return nil
	}
	render.List(a.Out, ready)
	return nil
}

// ListOpts filters list output.
type ListOpts struct {
	Label  string
	Epic   int
	Closed bool
}

// List shows issues, open by default, filtered by label or epic membership.
func (a *App) List(ctx context.Context, opts ListOpts) error {
	states := openStates
	if opts.Closed {
		states = []string{"CLOSED"}
	}
	if opts.Epic > 0 {
		// Children of an epic are interesting in both states: progress
		// means seeing what's done, not just what's left.
		states = allStates
	}
	issues, err := a.Client.ListIssues(ctx, states)
	if err != nil {
		return err
	}
	var out []model.Issue
	for _, i := range issues {
		if opts.Label != "" && !slices.Contains(i.Labels, opts.Label) {
			continue
		}
		if opts.Epic > 0 && (i.Parent == nil || i.Parent.Number != opts.Epic) {
			continue
		}
		if opts.Epic > 0 && opts.Closed && i.IsOpen() {
			continue
		}
		out = append(out, i)
	}
	model.SortForList(out)
	if a.JSON {
		return render.JSONList(a.Out, out)
	}
	if len(out) == 0 {
		a.printf("no issues\n")
		return nil
	}
	render.List(a.Out, out)
	return nil
}

// Show prints one issue in full: body, deps, parent, children, comments.
func (a *App) Show(ctx context.Context, number int) error {
	issue, err := a.Client.GetIssue(ctx, number)
	if err != nil {
		return err
	}
	if a.JSON {
		return render.JSONIssue(a.Out, issue)
	}
	render.Show(a.Out, issue)
	return nil
}

// Triage lists open issues missing their priority or type label, oldest
// first — work through them with `issues set`.
func (a *App) Triage(ctx context.Context) error {
	issues, err := a.Client.ListIssues(ctx, openStates)
	if err != nil {
		return err
	}
	untriaged := model.UntriagedIssues(issues)
	if a.JSON {
		return render.JSONList(a.Out, untriaged)
	}
	if len(untriaged) == 0 {
		a.printf("no untriaged issues\n")
		return nil
	}
	render.List(a.Out, untriaged)
	return nil
}

// Prime emits the session-start context: static conventions, live state,
// contradictions.
func (a *App) Prime(ctx context.Context) error {
	issues, err := a.Client.ListIssues(ctx, openStates)
	if err != nil {
		return err
	}
	ready := model.Ready(issues)
	d := render.PrimeData{
		Repo:       a.Repo.String(),
		Ready:      ready,
		ReadyTotal: len(ready),
		OpenTotal:  len(issues),
		InProgress: model.InProgressIssues(issues),
		Epics:      model.Epics(issues),
		Warnings:   model.Warnings(issues),
		Untriaged:  len(model.UntriagedIssues(issues)),
	}
	if len(d.Ready) > primeReadyCap {
		d.Ready = d.Ready[:primeReadyCap]
	}
	if a.JSON {
		return render.JSONPrime(a.Out, d)
	}
	render.Prime(a.Out, conventions.PrimerStatic, d)
	return nil
}

// EpicStatus with number <= 0 lists all open epics with progress rollups;
// with a number it shows that epic's children.
func (a *App) EpicStatus(ctx context.Context, number int) error {
	if number <= 0 {
		issues, err := a.Client.ListIssues(ctx, openStates)
		if err != nil {
			return err
		}
		epics := model.Epics(issues)
		if a.JSON {
			return render.JSONList(a.Out, epics)
		}
		if len(epics) == 0 {
			a.printf("no epics\n")
			return nil
		}
		render.EpicList(a.Out, epics)
		return nil
	}
	// One fetch of both states resolves child titles without N+1 queries.
	issues, err := a.Client.ListIssues(ctx, allStates)
	if err != nil {
		return err
	}
	byNum := model.ByNumber(issues)
	epic, ok := byNum[number]
	if !ok {
		return genericErr("issue #%d not found in %s", number, a.Repo)
	}
	if !epic.IsEpic() {
		return genericErr("#%d has no sub-issues; not an epic", number)
	}
	if a.JSON {
		return render.JSONEpicStatus(a.Out, epic, byNum)
	}
	render.EpicStatus(a.Out, epic, byNum)
	return nil
}
