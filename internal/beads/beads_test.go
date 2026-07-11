package beads

import (
	"strings"
	"testing"
	"time"
)

const fixture = `{"_type":"issue","id":"wdb-2","title":"Second","description":"d2","status":"open","priority":1,"issue_type":"feature","created_at":"2026-05-02T00:00:00Z","dependencies":[{"issue_id":"wdb-2","depends_on_id":"wdb-1","type":"blocks","created_at":"2026-05-02T00:00:00Z"}]}
{"_type":"issue","id":"wdb-1","title":"First stale","status":"open","priority":2,"issue_type":"task","created_at":"2026-05-01T00:00:00Z"}

{"_type":"issue","id":"wdb-1","title":"First","status":"closed","priority":2,"issue_type":"task","created_at":"2026-05-01T00:00:00Z","closed_at":"2026-05-03T00:00:00Z","close_reason":"done"}
{"_type":"issue","id":"wdb-3.1","title":"Child","status":"open","priority":3,"issue_type":"task","created_at":"2026-05-04T00:00:00Z","dependencies":[{"issue_id":"wdb-3.1","depends_on_id":"wdb-3","type":"parent-child"}]}
{"_type":"note","id":"ignored"}`

func TestParse(t *testing.T) {
	got, err := Parse(strings.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("parsed %d beads: %+v", len(got), got)
	}
	// Sorted by created_at: wdb-1, wdb-2, wdb-3.1.
	if got[0].ID != "wdb-1" || got[1].ID != "wdb-2" || got[2].ID != "wdb-3.1" {
		t.Errorf("order = %s %s %s", got[0].ID, got[1].ID, got[2].ID)
	}
	// Last record wins for the duplicated ID.
	if got[0].Title != "First" || !got[0].Closed() || got[0].CloseReason != "done" {
		t.Errorf("last-wins failed: %+v", got[0])
	}
	if got[0].ClosedAt == nil || !got[0].ClosedAt.Equal(time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("closedAt = %v", got[0].ClosedAt)
	}
	if bb := got[1].BlockedBy(); len(bb) != 1 || bb[0] != "wdb-1" {
		t.Errorf("BlockedBy = %v", bb)
	}
	if got[2].Parent() != "wdb-3" {
		t.Errorf("Parent = %q", got[2].Parent())
	}
	if got[1].Parent() != "" || len(got[0].BlockedBy()) != 0 {
		t.Error("phantom relations")
	}
}

func TestParseMalformed(t *testing.T) {
	_, err := Parse(strings.NewReader("{\"_type\":\"issue\",\"id\":\"a\"}\n{broken"))
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("err = %v", err)
	}
	_, err = Parse(strings.NewReader(`{"_type":"issue","title":"no id"}`))
	if err == nil || !strings.Contains(err.Error(), "no id") {
		t.Errorf("err = %v", err)
	}
}

func TestParseEmpty(t *testing.T) {
	got, err := Parse(strings.NewReader(""))
	if err != nil || len(got) != 0 {
		t.Errorf("Parse(empty) = %v, %v", got, err)
	}
}
