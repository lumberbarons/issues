package main

import (
	"context"
	"fmt"
	"regexp"
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

// primerGlobalFlags are documented once in the primer as "All take --json",
// not repeated in every entry.
var primerGlobalFlags = map[string]bool{
	"json": true,
	"repo": true,
}

// omittedFlags are deliberately absent from the terse primer, keyed by
// command so an exemption for one command can't hide the same flag drifting
// undocumented on another.
var omittedFlags = map[string]bool{
	"create --edit":    true, // interactive, not for agents
	"start --force":    true, // escape hatch
	"apply --state":    true, // plumbing; the default is right
	"apply --throttle": true, // plumbing; the default is right
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

// primerCommandEntries splits the primer's Commands section into one entry
// per command, keyed by the qualified command name ("ready", "epic create").
// Scoping the checks to a command's own entry is what makes them honest: a
// whole-primer substring search would pass on incidental prose matches
// ("untriaged" contains "triage") or on the same flag documented for a
// different command.
func primerCommandEntries(t *testing.T) map[string]string {
	t.Helper()
	_, section, found := strings.Cut(conventions.PrimerStatic, "Commands:")
	if !found {
		t.Fatal("primer has no Commands section")
	}
	var entries []string
	for frag := range strings.SplitSeq(section, "|") {
		frag = strings.TrimSpace(frag)
		if frag == "" {
			continue
		}
		// A fragment starting with a dash is the tail of a bracketed
		// alternative ("[--completed | --duplicate-of M]"), not a command —
		// rejoin it with its entry.
		if strings.HasPrefix(frag, "-") && len(entries) > 0 {
			entries[len(entries)-1] += " " + frag
			continue
		}
		entries = append(entries, frag)
	}
	isWord := regexp.MustCompile(`^[a-z]+$`).MatchString
	out := map[string]string{}
	for _, e := range entries {
		fields := strings.Fields(e)
		key := fields[0]
		if len(fields) > 1 && isWord(fields[1]) {
			key += " " + fields[1] // subcommand entry, e.g. "epic create"
		}
		out[key] = e
	}
	return out
}

// TestPrimerMatchesCommandSurface is the cross-check that keeps PrimerStatic
// honest: every agent-facing command must have its own entry in the primer's
// Commands section, and every agent-facing flag must appear in that
// command's entry (or be an explicit deliberate omission), so the
// hand-written cheatsheet can't silently drift from the real surface.
func TestPrimerMatchesCommandSurface(t *testing.T) {
	entries := primerCommandEntries(t)
	for _, lf := range leaves(root(), "") {
		if setupCommands[lf.topName] {
			continue
		}
		qualified := lf.name
		if lf.topName != lf.name {
			qualified = lf.topName + " " + lf.name
		}
		entry, ok := entries[qualified]
		if !ok {
			t.Errorf("primer's Commands section omits %q", qualified)
			continue
		}
		for _, fl := range lf.flags {
			name := fl.Names()[0]
			if primerGlobalFlags[name] || omittedFlags[qualified+" --"+name] {
				continue
			}
			if !regexp.MustCompile(`--` + regexp.QuoteMeta(name) + `\b`).MatchString(entry) {
				t.Errorf("primer entry for %q omits flag --%s: %q", qualified, name, entry)
			}
		}
	}
}

// findCommand walks the command tree by name path (e.g. "epic", "status").
func findCommand(t *testing.T, cmd *ucli.Command, path ...string) *ucli.Command {
	t.Helper()
	for _, name := range path {
		var next *ucli.Command
		for _, c := range cmd.Commands {
			if c.Name == name {
				next = c
				break
			}
		}
		if next == nil {
			t.Fatalf("command %q not found under %q", name, cmd.Name)
		}
		cmd = next
	}
	return cmd
}

// TestRepoFlagReachesCommand covers #25: the global --repo must reach the
// command in either position. Before the fix, `issues --repo owner/name <cmd>`
// parsed without error but the leaf's own shadowing --repo flag stayed empty,
// so writes silently went to the git-remote-detected repo. Actions are swapped
// for capture functions so the real flag wiring is exercised without a GitHub
// client.
func TestRepoFlagReachesCommand(t *testing.T) {
	const want = "octo/hello"
	cases := []struct {
		name string
		path []string
		args []string
	}{
		{"read before subcommand", []string{"list"}, []string{"issues", "--repo", want, "list"}},
		{"read after subcommand", []string{"list"}, []string{"issues", "list", "--repo", want}},
		{"write before subcommand", []string{"create"}, []string{"issues", "--repo", want, "create", "--type", "task", "--title", "t"}},
		{"write after subcommand", []string{"create"}, []string{"issues", "create", "--repo", want, "--type", "task", "--title", "t"}},
		{"nested subcommand", []string{"epic", "status"}, []string{"issues", "--repo", want, "epic", "status"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := root()
			var got string
			findCommand(t, app, tc.path...).Action = func(_ context.Context, cmd *ucli.Command) error {
				got = cmd.String("repo")
				return nil
			}
			if err := app.Run(context.Background(), tc.args); err != nil {
				t.Fatalf("run %v: %v", tc.args, err)
			}
			if got != want {
				t.Errorf("run %v: repo = %q, want %q", tc.args, got, want)
			}
		})
	}
}

// TestJSONFlagReachesCommand: --json shares the same declared-once-at-root
// wiring as --repo and would be silently dropped by the same shadowing bug.
func TestJSONFlagReachesCommand(t *testing.T) {
	cases := []struct {
		name string
		path []string
		args []string
	}{
		{"before subcommand", []string{"ready"}, []string{"issues", "--json", "ready"}},
		{"after subcommand", []string{"ready"}, []string{"issues", "ready", "--json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := root()
			var got bool
			findCommand(t, app, tc.path...).Action = func(_ context.Context, cmd *ucli.Command) error {
				got = cmd.Bool("json")
				return nil
			}
			if err := app.Run(context.Background(), tc.args); err != nil {
				t.Fatalf("run %v: %v", tc.args, err)
			}
			if !got {
				t.Errorf("run %v: json = false, want true", tc.args)
			}
		})
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
