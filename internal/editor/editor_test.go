package editor

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestEditRoundTrip(t *testing.T) {
	var gotEditor, gotPath string
	fake := func(name string, args ...string) error {
		// args are: -c, <script>, sh, <tmp path>
		if name != "sh" || len(args) != 4 {
			t.Fatalf("unexpected invocation: %s %v", name, args)
		}
		gotEditor = args[1]
		gotPath = args[3]
		return os.WriteFile(gotPath, []byte("edited body"), 0o600)
	}
	got, err := edit("myed", fake, "seed text")
	if err != nil {
		t.Fatal(err)
	}
	if got != "edited body" {
		t.Errorf("content = %q, want edited body", got)
	}
	if !strings.HasPrefix(gotEditor, "myed ") {
		t.Errorf("editor not invoked: %q", gotEditor)
	}
	if !strings.Contains(gotEditor, `"$1"`) {
		t.Errorf("temp path not passed as a quoted argument: %q", gotEditor)
	}
	if _, err := os.Stat(gotPath); !os.IsNotExist(err) {
		t.Errorf("temp file not cleaned up: %v", err)
	}
}

func TestEditSeedsInitial(t *testing.T) {
	var seen string
	fake := func(name string, args ...string) error {
		data, err := os.ReadFile(args[3])
		if err != nil {
			return err
		}
		seen = string(data)
		return nil
	}
	if _, err := edit("ed", fake, "the seed"); err != nil {
		t.Fatal(err)
	}
	if seen != "the seed" {
		t.Errorf("editor saw %q, want the seed", seen)
	}
}

func TestEditRequiresEditor(t *testing.T) {
	_, err := edit("", nil, "x")
	if err == nil || !strings.Contains(err.Error(), "$EDITOR") {
		t.Errorf("err = %v", err)
	}
}

func TestEditPropagatesRunFailure(t *testing.T) {
	boom := errors.New("boom")
	_, err := edit("ed", func(string, ...string) error { return boom }, "x")
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
}
