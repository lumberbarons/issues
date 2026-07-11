package model

import (
	"reflect"
	"testing"
)

func blocked(n int, by ...int) Issue {
	i := open(n, "P2", "bug")
	for _, b := range by {
		i.BlockedBy = append(i.BlockedBy, Ref{Number: b, State: "OPEN"})
	}
	return i
}

func TestCyclesNone(t *testing.T) {
	issues := []Issue{blocked(1), blocked(2, 1), blocked(3, 1, 2)}
	if got := Cycles(issues); len(got) != 0 {
		t.Errorf("Cycles() = %v, want none", got)
	}
}

func TestCyclesThreeIssue(t *testing.T) {
	// A(3) blocked by B(4), B by C(5), C by A — the case GitHub accepts.
	issues := []Issue{blocked(3, 4), blocked(4, 5), blocked(5, 3)}
	got := Cycles(issues)
	want := [][]int{{3, 4, 5, 3}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Cycles() = %v, want %v", got, want)
	}
}

func TestCyclesReportedOnce(t *testing.T) {
	// Two entry points into the same cycle must not duplicate it.
	issues := []Issue{blocked(1, 2), blocked(2, 3), blocked(3, 2), blocked(4, 3)}
	got := Cycles(issues)
	want := [][]int{{2, 3, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Cycles() = %v, want %v", got, want)
	}
}

func TestCyclesTwoDistinct(t *testing.T) {
	issues := []Issue{blocked(1, 2), blocked(2, 1), blocked(5, 6), blocked(6, 5)}
	got := Cycles(issues)
	want := [][]int{{1, 2, 1}, {5, 6, 5}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Cycles() = %v, want %v", got, want)
	}
}

func TestCyclesIgnoresClosedMembers(t *testing.T) {
	// 1 <- 2 <- 3 <- 1 but 3 is closed: closed blockers are non-blocking.
	three := blocked(3, 1)
	three.State = "CLOSED"
	issues := []Issue{blocked(1, 2), blocked(2, 3), three}
	if got := Cycles(issues); len(got) != 0 {
		t.Errorf("Cycles() = %v, want none", got)
	}
	// Same shape but the *edge* to 3 is recorded closed.
	two := open(2, "P2", "bug")
	two.BlockedBy = []Ref{{Number: 3, State: "CLOSED"}}
	issues = []Issue{blocked(1, 2), two, blocked(3, 1)}
	if got := Cycles(issues); len(got) != 0 {
		t.Errorf("Cycles() with closed edge = %v, want none", got)
	}
}

func TestWouldCycleSelf(t *testing.T) {
	if got := WouldCycle(nil, 7, 7); !reflect.DeepEqual(got, []int{7, 7}) {
		t.Errorf("WouldCycle(self) = %v", got)
	}
}

func TestWouldCycleDirect(t *testing.T) {
	// 2 is blocked by 1; adding "1 blocked by 2" closes the loop.
	issues := []Issue{blocked(1), blocked(2, 1)}
	got := WouldCycle(issues, 1, 2)
	want := []int{1, 2, 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WouldCycle() = %v, want %v", got, want)
	}
}

func TestWouldCycleTransitive(t *testing.T) {
	// 2 blocked by 1, 3 blocked by 2; adding "1 blocked by 3" -> 1←3←2←1.
	issues := []Issue{blocked(1), blocked(2, 1), blocked(3, 2)}
	got := WouldCycle(issues, 1, 3)
	want := []int{1, 3, 2, 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WouldCycle() = %v, want %v", got, want)
	}
}

func TestWouldCycleSafe(t *testing.T) {
	issues := []Issue{blocked(1), blocked(2, 1), blocked(3, 2)}
	if got := WouldCycle(issues, 3, 1); got != nil {
		t.Errorf("WouldCycle() = %v, want nil", got)
	}
	if got := WouldCycle(issues, 4, 1); got != nil {
		t.Errorf("WouldCycle(new issue) = %v, want nil", got)
	}
}

func TestWarnings(t *testing.T) {
	multiPri := open(1, "P0", "P2", "bug")
	multiType := open(2, "P1", "bug", "task")
	epicInProgress := open(3, "P2", "in-progress")
	epicInProgress.SubIssuesTotal = 2
	epicInProgress.SubIssues = []Ref{{Number: 10, State: "OPEN"}, {Number: 11, State: "OPEN"}}
	closedContradiction := Issue{Number: 4, State: "CLOSED", Labels: []string{"P0", "P1"}}
	truncatedSubs := open(6, "P2", "bug")
	truncatedSubs.SubIssuesTotal = 60
	truncatedSubs.SubIssues = make([]Ref, 50)
	truncatedBlockers := open(7, "P2", "bug")
	truncatedBlockers.BlockedByTotal = 25
	truncatedBlockers.BlockedBy = []Ref{{Number: 1, State: "OPEN"}}

	issues := []Issue{multiPri, multiType, epicInProgress, closedContradiction,
		blocked(8, 9), blocked(9, 8), truncatedSubs, truncatedBlockers}
	got := Warnings(issues)
	want := []Warning{
		{Kind: WarnMultiPriority, Issue: 1},
		{Kind: WarnMultiType, Issue: 2},
		{Kind: WarnInProgressEpic, Issue: 3},
		{Kind: WarnDependencyCycle, Cycle: []int{8, 9, 8}},
		{Kind: WarnSubIssuesCapped, Issue: 6, Total: 60, Fetched: 50},
		{Kind: WarnBlockersCapped, Issue: 7, Total: 25, Fetched: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Warnings() =\n%v\nwant\n%v", got, want)
	}
}

func TestWarningsOfKind(t *testing.T) {
	ws := []Warning{
		{Kind: WarnMultiPriority, Issue: 1},
		{Kind: WarnDependencyCycle, Cycle: []int{2, 3, 2}},
		{Kind: WarnBlockersCapped, Issue: 4, Total: 25, Fetched: 1},
	}
	got := WarningsOfKind(ws, WarnDependencyCycle, WarnBlockersCapped)
	want := []Warning{
		{Kind: WarnDependencyCycle, Cycle: []int{2, 3, 2}},
		{Kind: WarnBlockersCapped, Issue: 4, Total: 25, Fetched: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WarningsOfKind() = %v, want %v", got, want)
	}
}

func TestWarningsCleanRepo(t *testing.T) {
	if got := Warnings([]Issue{blocked(1), blocked(2, 1)}); got != nil {
		t.Errorf("Warnings() = %v, want nil", got)
	}
}
