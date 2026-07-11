// Package beads parses a beads (bd) issues.jsonl snapshot — the canonical
// git-synced export, one JSON object per line. Parsing the file directly
// instead of shelling out to bd keeps migration free of a runtime
// dependency on bd and its CLI surface.
package beads

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

// Dependency is one edge in a bead's dependency list. The types seen in
// real databases are "parent-child" (DependsOnID is the parent) and
// "blocks" (IssueID is blocked by DependsOnID).
type Dependency struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
}

// Bead is one issue record from the snapshot.
type Bead struct {
	ID                 string       `json:"id"`
	Title              string       `json:"title"`
	Description        string       `json:"description"`
	Status             string       `json:"status"` // open | in_progress | closed
	Priority           int          `json:"priority"`
	IssueType          string       `json:"issue_type"` // bug | feature | task | chore | epic
	Assignee           string       `json:"assignee"`
	Labels             []string     `json:"labels"`
	CreatedAt          time.Time    `json:"created_at"`
	ClosedAt           *time.Time   `json:"closed_at"`
	CloseReason        string       `json:"close_reason"`
	Design             string       `json:"design"`
	AcceptanceCriteria string       `json:"acceptance_criteria"`
	Notes              string       `json:"notes"`
	Dependencies       []Dependency `json:"dependencies"`
	Type               string       `json:"_type"`
}

// Closed reports whether the bead is closed.
func (b Bead) Closed() bool { return b.Status == "closed" }

// Parent returns the parent bead ID from a parent-child dependency, or "".
func (b Bead) Parent() string {
	for _, d := range b.Dependencies {
		if d.Type == "parent-child" {
			return d.DependsOnID
		}
	}
	return ""
}

// BlockedBy returns the bead IDs this bead is blocked by.
func (b Bead) BlockedBy() []string {
	var out []string
	for _, d := range b.Dependencies {
		if d.Type == "blocks" {
			out = append(out, d.DependsOnID)
		}
	}
	return out
}

// Parse reads a JSONL snapshot. Blank lines are skipped, records that
// aren't issues are ignored, the last record wins when an ID repeats, and
// the result is sorted by creation time (then ID) so migration preserves
// history order.
func Parse(r io.Reader) ([]Bead, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	byID := map[string]Bead{}
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var b Bead
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if b.Type != "" && b.Type != "issue" {
			continue
		}
		if b.ID == "" {
			return nil, fmt.Errorf("line %d: record has no id", line)
		}
		byID[b.ID] = b
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	out := make([]Bead, 0, len(byID))
	for _, b := range byID {
		out = append(out, b)
	}
	sort.Slice(out, func(a, b int) bool {
		if !out[a].CreatedAt.Equal(out[b].CreatedAt) {
			return out[a].CreatedAt.Before(out[b].CreatedAt)
		}
		return out[a].ID < out[b].ID
	})
	return out, nil
}
