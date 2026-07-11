package model

import (
	"fmt"
	"strconv"
	"strings"
)

// Warnings reports contradictions — the only per-issue conditions prime and
// ready surface. Absences (untriaged issues) are not warnings. Order is
// deterministic: per-issue warnings in issue-number order, then cycles,
// then truncation notices.
func Warnings(issues []Issue) []string {
	var out []string
	for _, i := range issues {
		if !i.IsOpen() {
			continue
		}
		if _, multi := i.Priority(); multi {
			out = append(out, fmt.Sprintf("#%d has multiple priority labels; highest wins", i.Number))
		}
		if _, multi := i.Type(); multi {
			out = append(out, fmt.Sprintf("#%d has multiple type labels; first of %s wins", i.Number, strings.Join(Types, "|")))
		}
		if i.IsEpic() && i.InProgress() {
			out = append(out, fmt.Sprintf("#%d is an in-progress epic; epics are never worked directly", i.Number))
		}
	}
	out = append(out, CycleWarnings(issues)...)
	for _, i := range issues {
		if !i.IsOpen() {
			continue
		}
		if i.SubIssuesTotal > len(i.SubIssues) {
			out = append(out, fmt.Sprintf("#%d has %d sub-issues, only %d fetched; counts may be incomplete", i.Number, i.SubIssuesTotal, len(i.SubIssues)))
		}
		if i.BlockedByTotal > len(i.BlockedBy) {
			out = append(out, fmt.Sprintf("#%d has %d blockers, only %d fetched; ready may be wrong", i.Number, i.BlockedByTotal, len(i.BlockedBy)))
		}
	}
	return out
}

// CycleWarnings is the cycle subset of Warnings; ready emits only these.
func CycleWarnings(issues []Issue) []string {
	var out []string
	for _, cyc := range Cycles(issues) {
		out = append(out, "dependency cycle "+cyclePath(cyc)+": none will be ready")
	}
	return out
}

func cyclePath(cyc []int) string {
	parts := make([]string, len(cyc))
	for i, n := range cyc {
		parts[i] = "#" + strconv.Itoa(n)
	}
	return strings.Join(parts, " → ")
}
