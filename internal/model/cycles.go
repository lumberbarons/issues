package model

import (
	"strconv"
	"strings"
)

// Cycles finds dependency cycles among open issues, following blocked-by
// edges. GitHub only rejects self-blocks and direct two-issue cycles, so
// longer cycles can exist (created via web UI or raw API) and must be
// detected client-side: every member of a cycle has an open blocker, so a
// cycle silently excludes all its members from ready forever.
//
// Each cycle is returned once as a slice of issue numbers in edge order
// (a blocked-by b blocked-by c ... blocked-by a), starting from its
// smallest member.
func Cycles(issues []Issue) [][]int {
	adj := blockedByGraph(issues)

	const (
		white = 0 // unvisited
		gray  = 1 // on current DFS path
		black = 2 // done
	)
	color := make(map[int]int, len(adj))
	var path []int
	onPath := make(map[int]int) // number -> index in path
	var cycles [][]int
	seen := make(map[string]bool)

	var dfs func(n int)
	dfs = func(n int) {
		color[n] = gray
		onPath[n] = len(path)
		path = append(path, n)
		for _, m := range adj[n] {
			switch color[m] {
			case white:
				dfs(m)
			case gray:
				cyc := canonical(path[onPath[m]:])
				if key := cycleKey(cyc); !seen[key] {
					seen[key] = true
					cycles = append(cycles, cyc)
				}
			}
		}
		path = path[:len(path)-1]
		delete(onPath, n)
		color[n] = black
	}

	// Iterate in ascending number order for deterministic output.
	for _, i := range issues {
		if _, ok := adj[i.Number]; ok && color[i.Number] == white {
			dfs(i.Number)
		}
	}
	return cycles
}

// WouldCycle reports the cycle path that adding "issue blocked by blocker"
// would create, or nil if the edge is safe. The check is transitive: the
// edge closes a cycle iff blocker is already (transitively) blocked by
// issue. The returned path is the resulting cycle in edge order, starting
// at issue.
func WouldCycle(issues []Issue, issue, blocker int) []int {
	if issue == blocker {
		return []int{issue, issue}
	}
	adj := blockedByGraph(issues)
	// Find a path blocker -> ... -> issue through blocked-by edges.
	visited := map[int]bool{blocker: true}
	var dfs func(n int, path []int) []int
	dfs = func(n int, path []int) []int {
		if n == issue {
			return path
		}
		for _, m := range adj[n] {
			if !visited[m] {
				visited[m] = true
				if p := dfs(m, append(path, m)); p != nil {
					return p
				}
			}
		}
		return nil
	}
	p := dfs(blocker, []int{blocker})
	if p == nil {
		return nil
	}
	// p is blocker -> ... -> issue; the new edge closes issue -> blocker.
	return append([]int{issue}, p...)
}

// blockedByGraph builds the adjacency map of blocked-by edges between open
// issues in the set. Edges to closed or unknown issues are non-blocking and
// dropped.
func blockedByGraph(issues []Issue) map[int][]int {
	open := make(map[int]bool, len(issues))
	for _, i := range issues {
		if i.IsOpen() {
			open[i.Number] = true
		}
	}
	adj := make(map[int][]int)
	for _, i := range issues {
		if !open[i.Number] {
			continue
		}
		for _, b := range i.BlockedBy {
			if b.IsOpen() && open[b.Number] {
				adj[i.Number] = append(adj[i.Number], b.Number)
			}
		}
	}
	return adj
}

// canonical rotates a cycle so its smallest member comes first and appends
// the closing member, e.g. [4 5 3] -> [3 4 5 3].
func canonical(cyc []int) []int {
	min := 0
	for i, n := range cyc {
		if n < cyc[min] {
			min = i
		}
	}
	out := make([]int, 0, len(cyc)+1)
	out = append(out, cyc[min:]...)
	out = append(out, cyc[:min]...)
	return append(out, out[0])
}

func cycleKey(cyc []int) string {
	var b strings.Builder
	for _, n := range cyc {
		b.WriteByte('/')
		b.WriteString(strconv.Itoa(n))
	}
	return b.String()
}
