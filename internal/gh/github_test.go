package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

// rewriteTransport sends every request to the test server, whatever host
// go-gh computed.
type rewriteTransport struct{ target *url.URL }

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

type recordedRequest struct {
	Method string
	Path   string
	Body   string
}

type fakeServer struct {
	*httptest.Server
	requests []recordedRequest
	// graphql maps a substring of the query to a response body.
	graphql map[string]string
	// rest maps "METHOD path" to status + response body.
	rest map[string]restResponse
}

type restResponse struct {
	status int
	body   string
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{graphql: map[string]string{}, rest: map[string]restResponse{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, recordedRequest{Method: r.Method, Path: r.URL.Path, Body: string(body)})
		if r.URL.Path == "/graphql" {
			var payload struct {
				Query string `json:"query"`
			}
			_ = json.Unmarshal(body, &payload)
			for substr, resp := range f.graphql {
				if strings.Contains(payload.Query, substr) {
					fmt.Fprint(w, resp)
					return
				}
			}
			t.Errorf("unexpected GraphQL query: %s", payload.Query)
			fmt.Fprint(w, `{"data":{}}`)
			return
		}
		key := r.Method + " " + r.URL.Path
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		if resp, ok := f.rest[key]; ok {
			w.WriteHeader(resp.status)
			fmt.Fprint(w, resp.body)
			return
		}
		t.Errorf("unexpected REST request: %s", key)
		w.WriteHeader(500)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeServer) client(t *testing.T) *GitHub {
	t.Helper()
	u, _ := url.Parse(f.URL)
	c, err := NewWithOptions(Repo{Owner: "o", Name: "r"}, api.ClientOptions{
		Host:      "github.com",
		AuthToken: "test-token",
		Transport: rewriteTransport{target: u},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func issueJSON(number int, extra string) string {
	base := fmt.Sprintf(`"id":"NODE%d","number":%d,"title":"Issue %d","state":"OPEN","stateReason":null,
		"createdAt":"2026-07-01T00:00:00Z",
		"labels":{"nodes":[{"name":"P2"},{"name":"bug"}]},
		"assignees":{"nodes":[]},
		"parent":null,
		"subIssues":{"totalCount":0,"nodes":[]},
		"subIssuesSummary":{"total":0,"completed":0},
		"blockedBy":{"totalCount":0,"nodes":[]}`, number, number, number)
	if extra != "" {
		base += "," + extra
	}
	return "{" + base + "}"
}

func TestViewer(t *testing.T) {
	f := newFakeServer(t)
	f.graphql["viewer"] = `{"data":{"viewer":{"login":"lumberbarons"}}}`
	login, err := f.client(t).Viewer(context.Background())
	if err != nil || login != "lumberbarons" {
		t.Fatalf("Viewer() = %q, %v", login, err)
	}
}

func TestListIssuesPaginates(t *testing.T) {
	f := newFakeServer(t)
	page := 0
	f.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(body, &payload)
		page++
		if page == 1 {
			if payload.Variables["cursor"] != nil {
				t.Errorf("first page had cursor %v", payload.Variables["cursor"])
			}
			fmt.Fprintf(w, `{"data":{"repository":{"issues":{"pageInfo":{"hasNextPage":true,"endCursor":"CUR"},"nodes":[%s]}}}}`, issueJSON(1, ""))
			return
		}
		if payload.Variables["cursor"] != "CUR" {
			t.Errorf("second page cursor = %v", payload.Variables["cursor"])
		}
		fmt.Fprintf(w, `{"data":{"repository":{"issues":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[%s]}}}}`, issueJSON(2, ""))
	})
	issues, err := f.client(t).ListIssues(context.Background(), []IssueState{StateOpen})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 || issues[0].Number != 1 || issues[1].Number != 2 {
		t.Fatalf("ListIssues() = %+v", issues)
	}
	if issues[0].ID != "NODE1" || issues[0].Labels[0] != "P2" {
		t.Errorf("issue fields not mapped: %+v", issues[0])
	}
}

func TestGetIssueMapsAllFields(t *testing.T) {
	f := newFakeServer(t)
	extra := `"body":"the body",
		"comments":{"nodes":[{"author":{"login":"alice"},"createdAt":"2026-07-02T00:00:00Z","body":"hi"},{"author":null,"createdAt":"2026-07-03T00:00:00Z","body":"ghost"}]}`
	node := `{"id":"NODE9","number":9,"title":"T","state":"OPEN","stateReason":null,
		"createdAt":"2026-07-01T00:00:00Z",
		"labels":{"nodes":[{"name":"P1"}]},
		"assignees":{"nodes":[{"login":"bob"}]},
		"parent":{"number":3,"state":"OPEN","title":"Epic: parent"},
		"subIssues":{"totalCount":2,"nodes":[{"number":10,"state":"OPEN"},{"number":11,"state":"CLOSED"}]},
		"subIssuesSummary":{"total":2,"completed":1},
		"blockedBy":{"totalCount":1,"nodes":[{"number":4,"state":"OPEN"}]},` + extra + `}`
	f.graphql["issue(number:"] = `{"data":{"repository":{"issue":` + node + `}}}`

	i, err := f.client(t).GetIssue(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if i.Body != "the body" || i.Parent.Number != 3 || i.ParentTitle != "Epic: parent" {
		t.Errorf("core fields: %+v", i)
	}
	if i.SubIssuesCompleted != 1 || i.SubIssuesTotal != 2 || len(i.SubIssues) != 2 {
		t.Errorf("sub-issue fields: %+v", i)
	}
	if len(i.BlockedBy) != 1 || i.BlockedBy[0].Number != 4 {
		t.Errorf("blockers: %+v", i.BlockedBy)
	}
	if len(i.Comments) != 2 || i.Comments[0].Author != "alice" || i.Comments[1].Author != "" {
		t.Errorf("comments: %+v", i.Comments)
	}
	if len(i.Assignees) != 1 || i.Assignees[0] != "bob" {
		t.Errorf("assignees: %+v", i.Assignees)
	}
}

func TestSearchIssues(t *testing.T) {
	f := newFakeServer(t)
	// The empty object is a non-issue node (a PR matched via user-supplied
	// qualifiers); it must be dropped, not mapped as issue #0.
	f.graphql["search(type: ISSUE"] = fmt.Sprintf(
		`{"data":{"search":{"issueCount":43,"nodes":[%s,{},%s]}}}`,
		issueJSON(7, ""), issueJSON(9, ""))
	issues, total, err := f.client(t).SearchIssues(context.Background(), "retry loop")
	if err != nil {
		t.Fatal(err)
	}
	if total != 43 {
		t.Errorf("total = %d, want 43", total)
	}
	if len(issues) != 2 || issues[0].Number != 7 || issues[1].Number != 9 {
		t.Fatalf("SearchIssues() = %+v", issues)
	}
	if issues[0].Labels[0] != "P2" {
		t.Errorf("issue fields not mapped: %+v", issues[0])
	}
	// The repo scope and is:issue must ride in the search string variable,
	// with the user's terms appended.
	req := f.requests[len(f.requests)-1]
	if !strings.Contains(req.Body, `repo:o/r is:issue retry loop`) {
		t.Errorf("search string not scoped: %s", req.Body)
	}
}

func TestGetIssueNotFound(t *testing.T) {
	f := newFakeServer(t)
	f.graphql["issue(number:"] = `{"data":{"repository":{"issue":null}}}`
	_, err := f.client(t).GetIssue(context.Background(), 404)
	if err == nil || !strings.Contains(err.Error(), "#404 not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateIssue(t *testing.T) {
	f := newFakeServer(t)
	f.rest["POST /repos/o/r/issues"] = restResponse{201, `{"node_id":"N1","number":42,"title":"New"}`}
	i, err := f.client(t).CreateIssue(context.Background(), "New", "body text", []string{"P2", "bug"})
	if err != nil || i.Number != 42 || i.ID != "N1" {
		t.Fatalf("CreateIssue() = %+v, %v", i, err)
	}
	req := f.requests[len(f.requests)-1]
	for _, want := range []string{`"title":"New"`, `"body":"body text"`, `"P2"`} {
		if !strings.Contains(req.Body, want) {
			t.Errorf("request body missing %s: %s", want, req.Body)
		}
	}
}

func TestSimpleRESTMutations(t *testing.T) {
	f := newFakeServer(t)
	f.rest["PATCH /repos/o/r/issues/5"] = restResponse{200, `{}`}
	f.rest["POST /repos/o/r/issues/5/labels"] = restResponse{200, `[]`}
	f.rest["POST /repos/o/r/issues/5/assignees"] = restResponse{201, `{}`}
	f.rest["DELETE /repos/o/r/issues/5/assignees"] = restResponse{200, `{}`}
	f.rest["POST /repos/o/r/issues/5/comments"] = restResponse{201, `{}`}
	c := f.client(t)
	ctx := context.Background()
	if err := c.EditTitle(ctx, 5, "Renamed"); err != nil {
		t.Error(err)
	}
	if err := c.AddLabels(ctx, 5, []string{"P1"}); err != nil {
		t.Error(err)
	}
	if err := c.AddAssignee(ctx, 5, "me"); err != nil {
		t.Error(err)
	}
	if err := c.RemoveAssignees(ctx, 5, []string{"other"}); err != nil {
		t.Error(err)
	}
	if err := c.Comment(ctx, 5, "a note"); err != nil {
		t.Error(err)
	}
	if len(f.requests) != 5 {
		t.Errorf("recorded %d requests", len(f.requests))
	}
}

func TestRemoveLabelToleratesMissing(t *testing.T) {
	f := newFakeServer(t)
	f.rest["DELETE /repos/o/r/issues/5/labels/P1"] = restResponse{200, `[]`}
	f.rest["DELETE /repos/o/r/issues/5/labels/gone"] = restResponse{404, `{"message":"Label does not exist"}`}
	c := f.client(t)
	if err := c.RemoveLabel(context.Background(), 5, "P1"); err != nil {
		t.Error(err)
	}
	if err := c.RemoveLabel(context.Background(), 5, "gone"); err != nil {
		t.Errorf("404 should be tolerated: %v", err)
	}
}

func TestGraphQLMutations(t *testing.T) {
	f := newFakeServer(t)
	// The mutation APIs resolve issue numbers to node IDs first.
	f.graphql["issue(number:"] = `{"data":{"repository":{"issue":{"id":"NODE"}}}}`
	f.graphql["closeIssue"] = `{"data":{"closeIssue":{"clientMutationId":null}}}`
	f.graphql["addBlockedBy"] = `{"data":{"addBlockedBy":{"clientMutationId":null}}}`
	f.graphql["removeBlockedBy"] = `{"data":{"removeBlockedBy":{"clientMutationId":null}}}`
	f.graphql["addSubIssue"] = `{"data":{"addSubIssue":{"clientMutationId":null}}}`
	f.graphql["removeSubIssue"] = `{"data":{"removeSubIssue":{"clientMutationId":null}}}`
	c := f.client(t)
	ctx := context.Background()
	if err := c.CloseIssue(ctx, 1, CloseNotPlanned); err != nil {
		t.Error(err)
	}
	if err := c.AddBlockedBy(ctx, 1, 2); err != nil {
		t.Error(err)
	}
	if err := c.RemoveBlockedBy(ctx, 1, 2); err != nil {
		t.Error(err)
	}
	if err := c.AddSubIssue(ctx, 1, 2, true); err != nil {
		t.Error(err)
	}
	if err := c.RemoveSubIssue(ctx, 1, 2); err != nil {
		t.Error(err)
	}
	// The close reason must ride in the variables as an enum value, not be
	// interpolated into the query text.
	var sawClose bool
	for _, r := range f.requests {
		if !strings.Contains(r.Body, "closeIssue(") {
			continue
		}
		sawClose = true
		if strings.Contains(r.Body, "stateReason: NOT_PLANNED") {
			t.Errorf("stateReason interpolated into query: %s", r.Body)
		}
		if !strings.Contains(r.Body, `"reason":"NOT_PLANNED"`) {
			t.Errorf("stateReason not sent as a variable: %s", r.Body)
		}
	}
	if !sawClose {
		t.Error("no closeIssue mutation was sent")
	}
}

func TestMutationResolvesNodeIDAndReportsMissing(t *testing.T) {
	f := newFakeServer(t)
	// Node-ID resolution finds no issue: the mutation must surface not-found
	// rather than sending a bad ID to the mutation.
	f.graphql["issue(number:"] = `{"data":{"repository":{"issue":null}}}`
	err := f.client(t).CloseIssue(context.Background(), 404, CloseCompleted)
	if err == nil || !strings.Contains(err.Error(), "#404 not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestNodeIDResolutionIsMemoized(t *testing.T) {
	f := newFakeServer(t)
	f.graphql["issue(number:"] = `{"data":{"repository":{"issue":{"id":"NODE"}}}}`
	f.graphql["addBlockedBy"] = `{"data":{"addBlockedBy":{"clientMutationId":null}}}`
	f.graphql["removeBlockedBy"] = `{"data":{"removeBlockedBy":{"clientMutationId":null}}}`
	c := f.client(t)
	ctx := context.Background()
	// Two mutations over the same pair need four node IDs but only two
	// distinct issues; each issue must be resolved once, not per edge.
	if err := c.AddBlockedBy(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveBlockedBy(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	resolutions := 0
	for _, r := range f.requests {
		if strings.Contains(r.Body, "issue(number:") {
			resolutions++
		}
	}
	if resolutions != 2 {
		t.Errorf("node-ID resolutions = %d, want 2 (one per distinct issue)", resolutions)
	}
}

func TestLabelsListAndCreate(t *testing.T) {
	f := newFakeServer(t)
	f.rest["GET /repos/o/r/labels?per_page=100&page=1"] = restResponse{200, `[{"name":"bug","color":"d73a4a","description":"x"}]`}
	f.rest["POST /repos/o/r/labels"] = restResponse{201, `{}`}
	c := f.client(t)
	labels, err := c.ListLabels(context.Background())
	if err != nil || len(labels) != 1 || labels[0].Name != "bug" {
		t.Fatalf("ListLabels() = %+v, %v", labels, err)
	}
	if err := c.CreateLabel(context.Background(), Label{Name: "P0", Color: "b60205", Description: "d"}); err != nil {
		t.Error(err)
	}
}

func TestGetIssueGraphQLNotResolve(t *testing.T) {
	f := newFakeServer(t)
	f.graphql["issue(number:"] = `{"data":{"repository":{"issue":null}},"errors":[{"type":"NOT_FOUND","message":"Could not resolve to an Issue with the number of 404."}]}`
	_, err := f.client(t).GetIssue(context.Background(), 404)
	if err == nil || !strings.Contains(err.Error(), "#404 not found in o/r") {
		t.Fatalf("err = %v", err)
	}
}

func TestGetIssueRepositoryNotFound(t *testing.T) {
	f := newFakeServer(t)
	// A missing/inaccessible repo fails to resolve at the repository path,
	// not the issue path — the message must blame the repo, not the issue.
	f.graphql["issue(number:"] = `{"data":{"repository":null},"errors":[{"type":"NOT_FOUND","path":["repository"],"message":"Could not resolve to a Repository with the name 'o/r'."}]}`
	_, err := f.client(t).GetIssue(context.Background(), 5)
	if err == nil || !strings.Contains(err.Error(), "repository o/r not found") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "issue #5") {
		t.Fatalf("repository error misreported as missing issue: %v", err)
	}
}

func TestViewerAuthError(t *testing.T) {
	f := newFakeServer(t)
	f.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"message":"Bad credentials"}`)
	})
	_, err := f.client(t).Viewer(context.Background())
	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthError, got %v", err)
	}
	if authErr.Unwrap() == nil {
		t.Error("AuthError should wrap the underlying error")
	}
}

func TestAuthErrorOn401(t *testing.T) {
	f := newFakeServer(t)
	f.rest["POST /repos/o/r/issues/5/comments"] = restResponse{401, `{"message":"Bad credentials"}`}
	err := f.client(t).Comment(context.Background(), 5, "x")
	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthError, got %v", err)
	}
	if !strings.Contains(authErr.Error(), "gh auth login") {
		t.Errorf("AuthError message: %v", authErr)
	}
}
