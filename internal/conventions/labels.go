// Package conventions holds the tool's opinions: the label set, the issue
// body template, and the static primer text.
package conventions

// Label describes one label in the bootstrap set created by `issues init`.
type Label struct {
	Name        string
	Color       string // hex, no leading #
	Description string
}

// Labels is the full convention label set. `init` creates the missing ones
// and updates color/description drift on existing ones is left alone —
// names are the contract, cosmetics are the repo owner's.
var Labels = []Label{
	{"P0", "b60205", "Critical: drop everything"},
	{"P1", "d93f0b", "High: next up"},
	{"P2", "fbca04", "Normal (default)"},
	{"P3", "0e8a16", "Low: nice to have"},
	{"P4", "c2e0c6", "Backlog: someday"},
	{"bug", "d73a4a", "Something isn't working"},
	{"enhancement", "a2eeef", "New feature or improvement"},
	{"task", "bfdadc", "Chore, refactor, or process work"},
	{"in-progress", "1d76db", "Actively being worked (claimed)"},
}
