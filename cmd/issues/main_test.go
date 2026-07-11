package main

import (
	"fmt"
	"strings"
	"testing"

	ucli "github.com/urfave/cli/v3"

	appcli "github.com/lumberbarons/issues/internal/cli"
	"github.com/lumberbarons/issues/internal/conventions"
	"github.com/lumberbarons/issues/internal/model"
)

// setupCommands are one-time/setup verbs the agent-facing primer deliberately
// omits; they aren't part of the issue-work loop it documents.
var setupCommands = map[string]bool{
	"prime":   true, // prime IS the primer; it doesn't list itself
	"init":    true,
	"hooks":   true,
	"migrate": true,
}

// omittedFlags are deliberately absent from the terse primer: the two global
// flags (documented once as "All take --json") plus escape hatches an agent
// doesn't need in the common loop.
var omittedFlags = map[string]bool{
	"json":  true,
	"repo":  true,
	"edit":  true, // create --edit: interactive, not for agents
	"force": true, // start --force: escape hatch
}

type leaf struct {
	name    string
	topName string
	flags   []ucli.Flag
}

func leaves(cmd *ucli.Command, top string) []leaf {
	if len(cmd.Commands) == 0 {
		return []leaf{{name: cmd.Name, topName: top, flags: cmd.Flags}}
	}
	var out []leaf
	for _, sub := range cmd.Commands {
		t := top
		if t == "" {
			t = sub.Name
		}
		out = append(out, leaves(sub, t)...)
	}
	return out
}

// TestPrimerMatchesCommandSurface is the cross-check that keeps PrimerStatic
// honest: every agent-facing command and flag defined in the command tree
// must appear in the primer (or be an explicit deliberate omission), so the
// hand-written cheatsheet can't silently drift from the real surface.
func TestPrimerMatchesCommandSurface(t *testing.T) {
	primer := conventions.PrimerStatic
	for _, lf := range leaves(root(), "") {
		if setupCommands[lf.topName] {
			continue
		}
		if !strings.Contains(primer, lf.name) {
			t.Errorf("primer omits command %q", lf.name)
		}
		for _, fl := range lf.flags {
			name := fl.Names()[0]
			if omittedFlags[name] {
				continue
			}
			if !strings.Contains(primer, "--"+name) {
				t.Errorf("primer omits flag --%s (on %q)", name, lf.name)
			}
		}
	}
}

// TestPrimerFactsTrackCode ties the primer's load-bearing numbers to the code
// that defines them, so changing a value without updating the prose fails.
func TestPrimerFactsTrackCode(t *testing.T) {
	primer := conventions.PrimerStatic
	if want := fmt.Sprintf("exit %d", appcli.ExitClaimed); !strings.Contains(primer, want) {
		t.Errorf("primer must state the claimed exit code as %q", want)
	}
	if want := "default " + model.DefaultPriority.String(); !strings.Contains(primer, want) {
		t.Errorf("primer must state the default priority as %q", want)
	}
}
