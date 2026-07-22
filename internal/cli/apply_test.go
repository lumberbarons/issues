package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// applyFixture covers the plan shapes: an epic with a body, a child by
// local-id parent, and an id-less entry referencing both a local id and an
// existing issue number, with a discovered-from link.
const applyFixture = `{"id":"epic1","title":"Voltgo support","type":"epic","priority":"P1","body":"### Goal\n\nstuff"}
{"id":"scaffold","title":"Scaffold","type":"task","parent":"epic1","areas":["ble"]}
{"title":"Collector","type":"enhancement","priority":"P3","parent":"epic1","blocked-by":["scaffold",42],"discovered-from":7}
`

func applySetup(t *testing.T, fixture string) (*fakeClient, *App, ApplyOpts) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "plan.jsonl")
	if err := os.WriteFile(file, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFake(issue(42, "Existing dep", "P2", "task"))
	app, _, _ := newApp(f)
	return f, app, ApplyOpts{File: file, StatePath: filepath.Join(dir, "state.json")}
}

func TestApplyCreatesAndWires(t *testing.T) {
	f, app, opts := applySetup(t, applyFixture)
	if err := app.Apply(ctx, opts); err != nil {
		t.Fatal(err)
	}
	// Creation in file order: epic (101), scaffold (102), collector (103).
	epic := f.byNumber(101)
	if epic == nil || epic.Title != "Epic: Voltgo support" {
		t.Fatalf("epic = %+v", epic)
	}
	if got, _ := epic.Type(); got != "" {
		t.Errorf("epic has type label: %v", epic.Labels)
	}
	if !slices.Contains(epic.Labels, "P1") || !strings.Contains(epic.Body, "### Goal") {
		t.Errorf("epic labels = %v body = %q", epic.Labels, epic.Body)
	}
	scaffold := f.byNumber(102)
	if !slices.Contains(scaffold.Labels, "P2") || !slices.Contains(scaffold.Labels, "task") || !slices.Contains(scaffold.Labels, "ble") {
		t.Errorf("scaffold labels = %v", scaffold.Labels)
	}
	if scaffold.Parent == nil || scaffold.Parent.Number != 101 {
		t.Errorf("scaffold parent = %v", scaffold.Parent)
	}
	collector := f.byNumber(103)
	if collector.Parent == nil || collector.Parent.Number != 101 {
		t.Errorf("collector parent = %v", collector.Parent)
	}
	blockers := []int{}
	for _, b := range collector.BlockedBy {
		blockers = append(blockers, b.Number)
	}
	if !slices.Contains(blockers, 102) || !slices.Contains(blockers, 42) {
		t.Errorf("collector blockedBy = %v", blockers)
	}
	if !strings.Contains(collector.Body, "Discovered while working on #7") {
		t.Errorf("collector body = %q", collector.Body)
	}
	// The id-less entry is checkpointed under its line key.
	data, _ := os.ReadFile(opts.StatePath)
	var state map[string]int
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state["epic1"] != 101 || state["scaffold"] != 102 || state["line:3"] != 103 {
		t.Errorf("state = %v", state)
	}
}

func TestApplyComposesSectionBodies(t *testing.T) {
	fixture := `{"title":"T","type":"task","goal":"Ship it","done-when":["tests pass"]}` + "\n"
	f, app, opts := applySetup(t, fixture)
	if err := app.Apply(ctx, opts); err != nil {
		t.Fatal(err)
	}
	want := "### Goal\n\nShip it\n\n### Done when\n\n- [ ] tests pass"
	if got := f.byNumber(101).Body; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestApplyDryRun(t *testing.T) {
	f, app, opts := applySetup(t, applyFixture)
	opts.DryRun = true
	if err := app.Apply(ctx, opts); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Errorf("dry run made API calls: %v", f.calls)
	}
	if _, err := os.Stat(opts.StatePath); !os.IsNotExist(err) {
		t.Error("dry run wrote state")
	}
	out := app.Out.(interface{ String() string }).String()
	for _, want := range []string{
		"create: epic1 as P1 epic  Epic: Voltgo support",
		"create: scaffold as P2 task  Scaffold  [areas ble; parent epic1]",
		"create: line:3 as P3 enhancement  Collector  [parent epic1; blocked by scaffold #42]",
		"dry run: 3 issues would be created",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan missing %q:\n%s", want, out)
		}
	}
}

func TestApplyResume(t *testing.T) {
	f, app, opts := applySetup(t, applyFixture)
	// Pretend the epic was already created as #55.
	f.issues = append(f.issues, issue(55, "Epic: Voltgo support", "P1"))
	f.issues[len(f.issues)-1].ID = "ID55"
	if err := os.WriteFile(opts.StatePath, []byte(`{"epic1": 55}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app.Apply(ctx, opts); err != nil {
		t.Fatal(err)
	}
	for _, i := range f.issues {
		if i.Title == "Epic: Voltgo support" && i.Number != 55 {
			t.Errorf("epic recreated as #%d", i.Number)
		}
	}
	// The children's parent edges must point at the pre-existing #55.
	scaffold := f.byNumber(101)
	if scaffold.Parent == nil || scaffold.Parent.Number != 55 {
		t.Errorf("scaffold parent = %v", scaffold.Parent)
	}
}

func TestApplyResumesAfterCreateFailure(t *testing.T) {
	f, app, opts := applySetup(t, applyFixture)
	// The API dies on the second create: the first entry's mapping must
	// already be on disk, and a rerun must pick up where the crash left off.
	f.failAfter["CreateIssue"] = failPoint{calls: 1, err: errors.New("boom")}
	err := app.Apply(ctx, opts)
	if err == nil || !strings.Contains(err.Error(), "rerun to resume") {
		t.Fatalf("err = %v", err)
	}
	data, readErr := os.ReadFile(opts.StatePath)
	if readErr != nil {
		t.Fatalf("no state persisted before the crash: %v", readErr)
	}
	var state map[string]int
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if len(state) != 1 || state["epic1"] != 101 {
		t.Fatalf("state after crash = %v", state)
	}

	delete(f.failAfter, "CreateIssue")
	if err := app.Apply(ctx, opts); err != nil {
		t.Fatal(err)
	}
	if len(f.issues) != 4 { // #42 plus the three plan entries
		t.Errorf("resume duplicated issues: %d total", len(f.issues))
	}
	data, _ = os.ReadFile(opts.StatePath)
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if len(state) != 3 || state["epic1"] != 101 {
		t.Errorf("state after resume = %v", state)
	}
}

func TestApplyRefusesCorruptState(t *testing.T) {
	f, app, opts := applySetup(t, applyFixture)
	if err := os.WriteFile(opts.StatePath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A corrupt state file must abort, not be treated as "nothing created
	// yet" — that would duplicate every already-created issue.
	err := app.Apply(ctx, opts)
	if err == nil || !strings.Contains(err.Error(), "not a valid resume-state file") {
		t.Fatalf("err = %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("API calls made despite corrupt state: %v", f.calls)
	}
}

func TestApplyInvalidPlan(t *testing.T) {
	f, app, opts := applySetup(t, `{"title":"x","type":"task","parent":"nope"}`+"\n")
	err := app.Apply(ctx, opts)
	exitCode(t, err, ExitGeneric)
	if err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Fatalf("err = %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("API calls made despite invalid plan: %v", f.calls)
	}
}

func TestApplyMissingFile(t *testing.T) {
	_, app, opts := applySetup(t, applyFixture)
	opts.File = "/nonexistent.jsonl"
	exitCode(t, app.Apply(ctx, opts), ExitGeneric)
}

func TestApplyNoFileArg(t *testing.T) {
	_, app, opts := applySetup(t, applyFixture)
	opts.File = ""
	exitCode(t, app.Apply(ctx, opts), ExitUsage)
}

func TestApplyEmptyPlan(t *testing.T) {
	f, app, opts := applySetup(t, "\n")
	if err := app.Apply(ctx, opts); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Errorf("calls made: %v", f.calls)
	}
	out := app.Out.(interface{ String() string }).String()
	if !strings.Contains(out, "nothing to apply") {
		t.Errorf("output = %q", out)
	}
}

func TestApplyDefaultStatePath(t *testing.T) {
	_, app, opts := applySetup(t, applyFixture)
	opts.StatePath = ""
	if err := app.Apply(ctx, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(opts.File + ".state.json"); err != nil {
		t.Errorf("default state file not written: %v", err)
	}
}

func TestApplyCreatesLabels(t *testing.T) {
	f, app, opts := applySetup(t, applyFixture)
	if err := app.Apply(ctx, opts); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, l := range f.labels {
		names[l.Name] = true
	}
	for _, want := range []string{"P0", "P4", "bug", "enhancement", "task", "in-progress", "ble"} {
		if !names[want] {
			t.Errorf("label %q not ensured", want)
		}
	}
}

func TestApplyFailedEdgeWarns(t *testing.T) {
	// A numeric blocked-by pointing at an issue that doesn't exist fails at
	// the API; the create must survive and the edge failure must warn.
	f, app, opts := applySetup(t, `{"title":"x","type":"task","blocked-by":[999]}`+"\n")
	if err := app.Apply(ctx, opts); err != nil {
		t.Fatal(err)
	}
	if f.byNumber(101) == nil {
		t.Fatal("issue not created")
	}
	errOut := app.ErrOut.(interface{ String() string }).String()
	if !strings.Contains(errOut, "blocked-by edge") {
		t.Errorf("no edge warning: %q", errOut)
	}
}

func TestApplyJSON(t *testing.T) {
	_, app, opts := applySetup(t, applyFixture)
	app.JSON = true
	if err := app.Apply(ctx, opts); err != nil {
		t.Fatal(err)
	}
	out := app.Out.(interface{ Bytes() []byte }).Bytes()
	var got struct {
		Created int            `json:"created"`
		Wired   int            `json:"wired"`
		Mapping map[string]int `json:"mapping"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// Wired: two parent edges plus two blockers.
	if got.Created != 3 || got.Wired != 4 || len(got.Mapping) != 3 {
		t.Errorf("summary = %+v", got)
	}
}
