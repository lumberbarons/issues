package model

import "slices"

// WarningKind identifies a contradiction or data-completeness notice, so
// callers can filter by cause and renderers can format the message without
// parsing prose.
type WarningKind string

const (
	WarnMultiPriority   WarningKind = "multiple-priority-labels"
	WarnMultiType       WarningKind = "multiple-type-labels"
	WarnInProgressEpic  WarningKind = "in-progress-epic"
	WarnDependencyCycle WarningKind = "dependency-cycle"
	WarnSubIssuesCapped WarningKind = "sub-issues-truncated"
	WarnBlockersCapped  WarningKind = "blockers-truncated"
)

// Warning is a structured contradiction or completeness notice. Issue names
// the subject (0 for a cycle, which spans several); Cycle carries the member
// path for WarnDependencyCycle; Total and Fetched carry the server-side and
// fetched counts for the truncation kinds. Message formatting lives in the
// render package, not here.
type Warning struct {
	Kind    WarningKind
	Issue   int
	Cycle   []int
	Total   int
	Fetched int
}

// Warnings reports contradictions and data-completeness notices — the
// per-issue conditions prime and ready surface. Absences (untriaged issues)
// are not warnings. Order is deterministic given the input order:
// per-issue warnings first, then cycles, then truncation notices.
func Warnings(issues []Issue) []Warning {
	var out []Warning
	for _, i := range issues {
		if !i.IsOpen() {
			continue
		}
		if _, multi := i.Priority(); multi {
			out = append(out, Warning{Kind: WarnMultiPriority, Issue: i.Number})
		}
		if _, multi := i.Type(); multi {
			out = append(out, Warning{Kind: WarnMultiType, Issue: i.Number})
		}
		if i.IsEpic() && i.InProgress() {
			out = append(out, Warning{Kind: WarnInProgressEpic, Issue: i.Number})
		}
	}
	for _, cyc := range Cycles(issues) {
		out = append(out, Warning{Kind: WarnDependencyCycle, Cycle: cyc})
	}
	for _, i := range issues {
		if !i.IsOpen() {
			continue
		}
		if i.SubIssuesTotal > len(i.SubIssues) {
			out = append(out, Warning{Kind: WarnSubIssuesCapped, Issue: i.Number, Total: i.SubIssuesTotal, Fetched: len(i.SubIssues)})
		}
		if i.BlockedByTotal > len(i.BlockedBy) {
			out = append(out, Warning{Kind: WarnBlockersCapped, Issue: i.Number, Total: i.BlockedByTotal, Fetched: len(i.BlockedBy)})
		}
	}
	return out
}

// WarningsOfKind returns the subset of ws matching any of kinds, preserving
// order — each command surfaces the kinds relevant to its output.
func WarningsOfKind(ws []Warning, kinds ...WarningKind) []Warning {
	var out []Warning
	for _, w := range ws {
		if slices.Contains(kinds, w.Kind) {
			out = append(out, w)
		}
	}
	return out
}
