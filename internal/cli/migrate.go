package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lumberbarons/issues/internal/beads"
	"github.com/lumberbarons/issues/internal/conventions"
	"github.com/lumberbarons/issues/internal/gh"
	"github.com/lumberbarons/issues/internal/model"
	"github.com/lumberbarons/issues/internal/render"
)

// MigrateOpts configure the beads migration.
type MigrateOpts struct {
	// File is the beads issues.jsonl snapshot.
	File string
	// StatePath is where the beadID→issue-number mapping is persisted
	// after every create, making a failed run resumable.
	StatePath string
	// DryRun prints the plan without touching GitHub.
	DryRun bool
	// IncludeClosed migrates closed beads too (created, commented with the
	// close reason, then closed). Real databases are >95% closed, so this
	// is opt-in.
	IncludeClosed bool
	// Throttle is slept between writes to stay under GitHub's secondary
	// rate limits for content creation.
	Throttle time.Duration
}

// beadTypeLabels maps bead issue types to convention type labels; epics
// map to no type label (epic-ness is having sub-issues).
var beadTypeLabels = map[string]string{
	"bug":     "bug",
	"feature": "enhancement",
	"task":    "task",
	"chore":   "task",
	"epic":    "",
}

// MigrateBeads imports a beads snapshot as GitHub issues: create in
// history order, wire parents and blockers, then close what was closed.
func (a *App) MigrateBeads(ctx context.Context, opts MigrateOpts) error {
	f, err := os.Open(opts.File)
	if err != nil {
		return genericErr("cannot read beads snapshot: %v", err)
	}
	defer func() { _ = f.Close() }()
	all, err := beads.Parse(f)
	if err != nil {
		return genericErr("parsing %s: %v", opts.File, err)
	}

	var selected []beads.Bead
	skippedClosed := 0
	for _, b := range all {
		switch b.Status {
		case "open", "in_progress":
			selected = append(selected, b)
		case "closed":
			if opts.IncludeClosed {
				selected = append(selected, b)
			} else {
				skippedClosed++
			}
		default:
			a.warnf("%s has unknown status %q; migrating as open", b.ID, b.Status)
			selected = append(selected, b)
		}
	}
	if len(selected) == 0 {
		a.printf("nothing to migrate (%d closed beads skipped; use --include-closed)\n", skippedClosed)
		return nil
	}

	state, err := loadMigrationState(opts.StatePath)
	if err != nil {
		return err
	}

	if opts.DryRun {
		a.migrationPlan(selected, state, skippedClosed)
		return nil
	}

	if err := a.ensureMigrationLabels(ctx, selected); err != nil {
		return err
	}

	viewer := ""
	created, err := a.migrateCreate(ctx, selected, state, opts, &viewer)
	if err != nil {
		return err
	}
	wired, warned := a.migrateWire(ctx, selected, state, opts)
	closed := a.migrateClose(ctx, selected, state, opts)

	if a.JSON {
		return render.WriteJSON(a.Out, map[string]any{
			"created": created, "wired": wired, "closed": closed,
			"skippedClosed": skippedClosed, "warnings": warned,
			"mapping": state,
		})
	}
	a.printf("migrated %d beads: %d created, %d dependencies wired, %d closed", len(selected), created, wired, closed)
	if skippedClosed > 0 {
		a.printf(" (%d closed beads skipped; use --include-closed)", skippedClosed)
	}
	a.printf("\nmapping saved to %s\n", opts.StatePath)
	return nil
}

// migrationPlan prints what a real run would do.
func (a *App) migrationPlan(selected []beads.Bead, state map[string]int, skippedClosed int) {
	for _, b := range selected {
		if n, ok := state[b.ID]; ok {
			a.printf("already migrated: %s → #%d\n", b.ID, n)
			continue
		}
		line := fmt.Sprintf("create: %s as %s", b.ID, model.Priority(clampPriority(b.Priority)))
		if t := beadTypeLabels[b.IssueType]; t != "" {
			line += " " + t
		} else if b.IssueType == "epic" {
			line += " epic"
		}
		line += "  " + b.Title
		var marks []string
		if p := b.Parent(); p != "" {
			marks = append(marks, "parent "+p)
		}
		if blockers := b.BlockedBy(); len(blockers) > 0 {
			marks = append(marks, "blocked by "+strings.Join(blockers, " "))
		}
		if b.Status == "in_progress" {
			marks = append(marks, "in progress")
		}
		if b.Closed() {
			marks = append(marks, "then close")
		}
		if len(marks) > 0 {
			line += "  [" + strings.Join(marks, "; ") + "]"
		}
		a.printf("%s\n", line)
	}
	a.printf("dry run: %d beads would be migrated", len(selected))
	if skippedClosed > 0 {
		a.printf(" (%d closed skipped; use --include-closed)", skippedClosed)
	}
	a.printf("\n")
}

// ensureMigrationLabels creates the convention labels plus every area
// label the beads carry, so creates never reference a missing label.
func (a *App) ensureMigrationLabels(ctx context.Context, selected []beads.Bead) error {
	existing, err := a.Client.ListLabels(ctx)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, l := range existing {
		have[l.Name] = true
	}
	var want []gh.Label
	for _, l := range conventions.Labels {
		want = append(want, gh.Label{Name: l.Name, Color: l.Color, Description: l.Description})
	}
	seen := map[string]bool{}
	for _, b := range selected {
		for _, area := range areaLabels(b.Labels) {
			if !seen[area] {
				seen[area] = true
				want = append(want, gh.Label{Name: area, Color: "ededed", Description: "migrated from beads"})
			}
		}
	}
	for _, l := range want {
		if have[l.Name] {
			continue
		}
		if err := a.Client.CreateLabel(ctx, l); err != nil {
			return fmt.Errorf("creating label %q: %w", l.Name, err)
		}
	}
	return nil
}

func (a *App) migrateCreate(ctx context.Context, selected []beads.Bead, state map[string]int, opts MigrateOpts, viewer *string) (int, error) {
	created := 0
	for _, b := range selected {
		if n, ok := state[b.ID]; ok {
			a.progressf("already migrated: %s → #%d\n", b.ID, n)
			continue
		}
		labels := []string{model.Priority(clampPriority(b.Priority)).String()}
		if t := beadTypeLabels[b.IssueType]; t != "" {
			labels = append(labels, t)
		}
		if b.Status == "in_progress" {
			labels = append(labels, model.InProgressLabel)
		}
		labels = append(labels, areaLabels(b.Labels)...)

		title := b.Title
		if b.IssueType == "epic" && !strings.HasPrefix(title, conventions.EpicTitlePrefix) {
			title = conventions.EpicTitlePrefix + title
		}

		issue, err := a.Client.CreateIssue(ctx, title, beadBody(b), labels)
		if err != nil {
			return created, fmt.Errorf("creating %s (rerun to resume): %w", b.ID, err)
		}
		state[b.ID] = issue.Number
		if err := saveMigrationState(opts.StatePath, state); err != nil {
			return created, err
		}
		created++
		a.progressf("created #%d from %s: %s\n", issue.Number, b.ID, b.Title)
		if b.Status == "in_progress" {
			if *viewer == "" {
				v, err := a.Client.Viewer(ctx)
				if err != nil {
					return created, err
				}
				*viewer = v
			}
			if err := a.Client.AddAssignee(ctx, issue.Number, *viewer); err != nil {
				a.warnf("assigning #%d: %v", issue.Number, err)
			}
		}
		sleep(opts.Throttle)
	}
	return created, nil
}

// migrateWire connects parents and blockers. Failures warn and continue —
// on resume the edges are retried, and a duplicate edge is harmless.
func (a *App) migrateWire(ctx context.Context, selected []beads.Bead, state map[string]int, opts MigrateOpts) (wired int, warnings []string) {
	ids := map[int]string{} // issue number -> node ID, fetched lazily
	nodeID := func(number int) (string, error) {
		if id, ok := ids[number]; ok {
			return id, nil
		}
		issue, err := a.Client.GetIssue(ctx, number)
		if err != nil {
			return "", err
		}
		ids[number] = issue.ID
		return issue.ID, nil
	}
	warn := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		warnings = append(warnings, msg)
		a.warnf("%s", msg)
	}
	edge := func(fromBead, toBead, kind string, connect func(fromID, toID string) error) {
		from, ok := state[fromBead]
		if !ok {
			return
		}
		to, ok := state[toBead]
		if !ok {
			warn("%s of %s not migrated, %s edge dropped", toBead, fromBead, kind)
			return
		}
		fromID, err := nodeID(from)
		if err == nil {
			var toID string
			if toID, err = nodeID(to); err == nil {
				err = connect(fromID, toID)
			}
		}
		if err != nil {
			warn("%s edge #%d→#%d: %v", kind, from, to, err)
			return
		}
		wired++
		sleep(opts.Throttle)
	}
	for _, b := range selected {
		if p := b.Parent(); p != "" {
			edge(b.ID, p, "parent", func(childID, parentID string) error {
				return a.Client.AddSubIssue(ctx, parentID, childID, true)
			})
		}
		for _, blocker := range b.BlockedBy() {
			edge(b.ID, blocker, "blocked-by", func(issueID, blockerID string) error {
				return a.Client.AddBlockedBy(ctx, issueID, blockerID)
			})
		}
	}
	return wired, warnings
}

// migrateClose closes migrated beads that were closed, commenting the
// close reason first. Tolerant: re-closing on resume just warns.
func (a *App) migrateClose(ctx context.Context, selected []beads.Bead, state map[string]int, opts MigrateOpts) int {
	closed := 0
	for _, b := range selected {
		if !b.Closed() {
			continue
		}
		number, ok := state[b.ID]
		if !ok {
			continue
		}
		issue, err := a.Client.GetIssue(ctx, number)
		if err != nil {
			a.warnf("closing #%d: %v", number, err)
			continue
		}
		if !issue.IsOpen() {
			continue
		}
		if b.CloseReason != "" {
			if err := a.Client.Comment(ctx, number, b.CloseReason); err != nil {
				a.warnf("close comment on #%d: %v", number, err)
			}
		}
		if err := a.Client.CloseIssue(ctx, issue.ID, "COMPLETED"); err != nil {
			a.warnf("closing #%d: %v", number, err)
			continue
		}
		closed++
		sleep(opts.Throttle)
	}
	return closed
}

// beadBody assembles the issue body from the bead's prose fields, with a
// provenance footer carrying what GitHub can't store natively.
func beadBody(b beads.Bead) string {
	var parts []string
	if s := strings.TrimSpace(b.Description); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(b.Design); s != "" {
		parts = append(parts, "### Design\n\n"+s)
	}
	if s := strings.TrimSpace(b.AcceptanceCriteria); s != "" {
		parts = append(parts, "### Done when\n\n"+s)
	}
	if s := strings.TrimSpace(b.Notes); s != "" {
		parts = append(parts, "### Notes\n\n"+s)
	}
	footer := fmt.Sprintf("Migrated from beads `%s` (created %s", b.ID, b.CreatedAt.Format("2006-01-02"))
	if b.ClosedAt != nil {
		footer += ", closed " + b.ClosedAt.Format("2006-01-02")
	}
	footer += ")"
	parts = append(parts, "---\n"+footer)
	return strings.Join(parts, "\n\n")
}

// areaLabels filters bead labels that would collide with the convention
// labels (priority, type, in-progress) — those are derived, not copied.
func areaLabels(labels []string) []string {
	var out []string
	for _, l := range labels {
		if _, isPriority := model.ParsePriority(l); isPriority || model.IsType(l) || l == model.InProgressLabel {
			continue
		}
		out = append(out, l)
	}
	return out
}

func clampPriority(p int) int {
	if p < 0 {
		return 0
	}
	if p > 4 {
		return 4
	}
	return p
}

func loadMigrationState(path string) (map[string]int, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, err
	}
	var state map[string]int
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("%s is not a valid migration state file: %w", path, err)
	}
	return state, nil
}

func saveMigrationState(path string, state map[string]int) error {
	data, err := json.MarshalIndent(state, "", "  ") // map keys marshal sorted
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func sleep(d time.Duration) {
	if d > 0 {
		time.Sleep(d)
	}
}
