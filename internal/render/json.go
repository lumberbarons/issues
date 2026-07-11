package render

import (
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/lumberbarons/issues/internal/model"
)

// IssueJSON is the stable flat schema for --json output: deps as number
// arrays, GraphQL shapes hidden.
type IssueJSON struct {
	Number             int           `json:"number"`
	Title              string        `json:"title"`
	State              string        `json:"state"`
	Priority           *string       `json:"priority"`
	Type               *string       `json:"type"`
	Areas              []string      `json:"areas"`
	Assignees          []string      `json:"assignees"`
	Epic               bool          `json:"epic"`
	InProgress         bool          `json:"inProgress"`
	Untriaged          bool          `json:"untriaged"`
	Parent             *int          `json:"parent"`
	BlockedBy          []int         `json:"blockedBy"`
	OpenBlockers       []int         `json:"openBlockers"`
	SubIssues          []int         `json:"subIssues"`
	SubIssuesCompleted int           `json:"subIssuesCompleted"`
	CreatedAt          time.Time     `json:"createdAt"`
	Body               string        `json:"body,omitempty"`
	Comments           []CommentJSON `json:"comments,omitempty"`
}

// CommentJSON is one recent comment in show --json.
type CommentJSON struct {
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
	Body      string    `json:"body"`
}

// ToJSON converts an issue to the flat schema. Body and comments are only
// carried when withDetail is set (show).
func ToJSON(i model.Issue, withDetail bool) IssueJSON {
	out := IssueJSON{
		Number:             i.Number,
		Title:              i.Title,
		State:              strings.ToLower(i.State),
		Areas:              emptyNotNull(i.Areas()),
		Assignees:          emptyNotNull(i.Assignees),
		Epic:               i.IsEpic(),
		InProgress:         i.InProgress(),
		Untriaged:          i.Untriaged(),
		BlockedBy:          refNumbers(i.BlockedBy),
		OpenBlockers:       emptyIntNotNull(i.OpenBlockers()),
		SubIssues:          refNumbers(i.SubIssues),
		SubIssuesCompleted: i.SubIssuesCompleted,
		CreatedAt:          i.CreatedAt,
	}
	if p, _ := i.Priority(); p != model.PriorityUnknown {
		s := p.String()
		out.Priority = &s
	}
	if t, _ := i.Type(); t != "" {
		out.Type = &t
	}
	if i.Parent != nil {
		n := i.Parent.Number
		out.Parent = &n
	}
	if withDetail {
		out.Body = i.Body
		for _, c := range i.Comments {
			out.Comments = append(out.Comments, CommentJSON{Author: c.Author, CreatedAt: c.CreatedAt, Body: c.Body})
		}
	}
	return out
}

// JSONList writes issues as NDJSON — one compact object per line. Unlike
// an array, it stays parseable under head/grep and agent output
// truncation, which is how list output actually gets consumed.
func JSONList(w io.Writer, issues []model.Issue) error {
	enc := json.NewEncoder(w)
	for _, i := range issues {
		if err := enc.Encode(ToJSON(i, false)); err != nil {
			return err
		}
	}
	return nil
}

// JSONIssue writes one issue with full detail.
func JSONIssue(w io.Writer, i model.Issue) error {
	return writeJSON(w, ToJSON(i, true))
}

// PrimeJSON is the structured form of the primer's live state.
type PrimeJSON struct {
	Repo       string      `json:"repo"`
	OpenTotal  int         `json:"openTotal"`
	ReadyTotal int         `json:"readyTotal"`
	Ready      []IssueJSON `json:"ready"`
	InProgress []IssueJSON `json:"inProgress"`
	Epics      []IssueJSON `json:"epics"`
	Untriaged  int         `json:"untriaged"`
	Warnings   []string    `json:"warnings"`
}

// JSONPrime writes the primer's live state as JSON (the static primer text
// is for context injection, not machine consumption).
func JSONPrime(w io.Writer, d PrimeData) error {
	out := PrimeJSON{
		Repo:       d.Repo,
		OpenTotal:  d.OpenTotal,
		ReadyTotal: d.ReadyTotal,
		Ready:      toJSONList(d.Ready),
		InProgress: toJSONList(d.InProgress),
		Epics:      toJSONList(d.Epics),
		Untriaged:  d.Untriaged,
		Warnings:   emptyNotNull(d.Warnings),
	}
	return writeJSON(w, out)
}

// EpicStatusJSON pairs an epic with its resolved children.
type EpicStatusJSON struct {
	Epic     IssueJSON   `json:"epic"`
	Children []IssueJSON `json:"children"`
}

// JSONEpicStatus writes one epic and its children, resolving each child
// from the fetched set where possible.
func JSONEpicStatus(w io.Writer, epic model.Issue, byNumber map[int]model.Issue) error {
	out := EpicStatusJSON{Epic: ToJSON(epic, false), Children: []IssueJSON{}}
	for _, ref := range epic.SubIssues {
		if child, ok := byNumber[ref.Number]; ok {
			out.Children = append(out.Children, ToJSON(child, false))
		} else {
			out.Children = append(out.Children, IssueJSON{
				Number: ref.Number, State: strings.ToLower(ref.State),
				Areas: []string{}, Assignees: []string{},
				BlockedBy: []int{}, OpenBlockers: []int{}, SubIssues: []int{},
			})
		}
	}
	return writeJSON(w, out)
}

func toJSONList(issues []model.Issue) []IssueJSON {
	out := make([]IssueJSON, len(issues))
	for idx, i := range issues {
		out[idx] = ToJSON(i, false)
	}
	return out
}

func refNumbers(refs []model.Ref) []int {
	out := make([]int, len(refs))
	for idx, r := range refs {
		out[idx] = r.Number
	}
	return out
}

func emptyNotNull(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func emptyIntNotNull(s []int) []int {
	if s == nil {
		return []int{}
	}
	return s
}

// WriteJSON writes any value in the standard indented form; for the odd
// command whose output isn't issue-shaped.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeJSON(w io.Writer, v any) error { return WriteJSON(w, v) }
