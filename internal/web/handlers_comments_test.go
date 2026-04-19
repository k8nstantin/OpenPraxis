package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"database/sql"

	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
)

// ---- harness ----------------------------------------------------------------

type commentsTestEnv struct {
	store  *comments.Store
	server *httptest.Server
}

func newCommentsTestEnv(t *testing.T) *commentsTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "comments.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if err := comments.InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	store := comments.NewStore(db)

	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()
	registerCommentsRoutes(api, store)

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		srv.Close()
		db.Close()
	})
	return &commentsTestEnv{store: store, server: srv}
}

func (e *commentsTestEnv) do(t *testing.T, method, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var buf io.Reader
	if body != nil {
		switch v := body.(type) {
		case []byte:
			buf = bytes.NewReader(v)
		default:
			b, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			buf = bytes.NewReader(b)
		}
	}
	req, err := http.NewRequest(method, e.server.URL+path, buf)
	if err != nil {
		t.Fatalf("req: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, out
}

func decodeErr(t *testing.T, body []byte) map[string]string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("decode error body: %v raw=%s", err, body)
	}
	return m
}

func decodeComment(t *testing.T, body []byte) commentView {
	t.Helper()
	var v commentView
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode comment: %v raw=%s", err, body)
	}
	return v
}

func decodeList(t *testing.T, body []byte) []commentView {
	t.Helper()
	var v listCommentsResponse
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode list: %v raw=%s", err, body)
	}
	return v.Comments
}

// ---- POST -------------------------------------------------------------------

func TestPOST_AddComment_ProductScope(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "POST", "/api/products/p1/comments", addCommentRequest{
		Author: "alice",
		Type:   string(comments.TypeUserNote),
		Body:   "hello",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d body=%s", resp.StatusCode, body)
	}
	c := decodeComment(t, body)
	if c.ID == "" || c.TargetType != "product" || c.TargetID != "p1" ||
		c.Author != "alice" || c.Type != "user_note" || c.Body != "hello" {
		t.Fatalf("bad comment: %+v", c)
	}
	if c.CreatedAt == 0 || c.CreatedAtISO == "" {
		t.Fatalf("missing timestamps: %+v", c)
	}
	if c.UpdatedAt != nil || c.UpdatedAtISO != "" {
		t.Fatalf("unexpected updated fields on create: %+v", c)
	}
}

func TestPOST_AddComment_ManifestScope(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "POST", "/api/manifests/m1/comments", addCommentRequest{
		Author: "bob", Type: string(comments.TypeDecision), Body: "chose plan X",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d body=%s", resp.StatusCode, body)
	}
	c := decodeComment(t, body)
	if c.TargetType != "manifest" || c.TargetID != "m1" || c.Type != "decision" {
		t.Fatalf("bad comment: %+v", c)
	}
}

func TestPOST_AddComment_TaskScope(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "POST", "/api/tasks/t1/comments", addCommentRequest{
		Author: "carol", Type: string(comments.TypeExecutionReview), Body: "ran fine",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d body=%s", resp.StatusCode, body)
	}
	c := decodeComment(t, body)
	if c.TargetType != "task" || c.Type != "execution_review" {
		t.Fatalf("bad comment: %+v", c)
	}
}

func TestPOST_RejectsEmptyBody(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "POST", "/api/products/p1/comments", addCommentRequest{
		Author: "alice", Type: string(comments.TypeUserNote), Body: "   ",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", resp.StatusCode, body)
	}
	if got := decodeErr(t, body)["code"]; got != "empty_body" {
		t.Fatalf("code: got %q", got)
	}
}

func TestPOST_RejectsUnknownType(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "POST", "/api/products/p1/comments", addCommentRequest{
		Author: "alice", Type: "not_a_type", Body: "hi",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", resp.StatusCode, body)
	}
	if got := decodeErr(t, body)["code"]; got != "unknown_type" {
		t.Fatalf("code: got %q", got)
	}
}

func TestPOST_RejectsEmptyAuthor(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "POST", "/api/products/p1/comments", addCommentRequest{
		Author: "", Type: string(comments.TypeUserNote), Body: "hi",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", resp.StatusCode, body)
	}
	if got := decodeErr(t, body)["code"]; got != "empty_author" {
		t.Fatalf("code: got %q", got)
	}
}

// ---- GET --------------------------------------------------------------------

func TestGET_ListComments_NewestFirst(t *testing.T) {
	env := newCommentsTestEnv(t)
	// Insert three comments directly; store assigns UUID v7 ids (monotonic).
	for i, body := range []string{"first", "second", "third"} {
		if _, err := env.store.Add(t.Context(), comments.TargetProduct, "p1",
			"alice", comments.TypeUserNote, body); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	resp, body := env.do(t, "GET", "/api/products/p1/comments", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	list := decodeList(t, body)
	if len(list) != 3 {
		t.Fatalf("len: got %d", len(list))
	}
	// Newest-first: "third", "second", "first".
	if list[0].Body != "third" || list[2].Body != "first" {
		t.Fatalf("order: got %+v", list)
	}
}

func TestGET_ListComments_TypeFilter(t *testing.T) {
	env := newCommentsTestEnv(t)
	_, _ = env.store.Add(t.Context(), comments.TargetProduct, "p1", "a",
		comments.TypeUserNote, "u1")
	_, _ = env.store.Add(t.Context(), comments.TargetProduct, "p1", "a",
		comments.TypeDecision, "d1")
	resp, body := env.do(t, "GET", "/api/products/p1/comments?type=decision", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	list := decodeList(t, body)
	if len(list) != 1 || list[0].Type != "decision" {
		t.Fatalf("filter: got %+v", list)
	}
}

func TestGET_ListComments_LimitAndCap(t *testing.T) {
	env := newCommentsTestEnv(t)
	for i := 0; i < 5; i++ {
		_, _ = env.store.Add(t.Context(), comments.TargetProduct, "p1", "a",
			comments.TypeUserNote, fmt.Sprintf("n%d", i))
	}
	resp, body := env.do(t, "GET", "/api/products/p1/comments?limit=2", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	if list := decodeList(t, body); len(list) != 2 {
		t.Fatalf("limit=2: got %d", len(list))
	}
	// limit=2000 should be capped at 1000 — store caps; endpoint must not
	// reject the value. Only 5 rows exist so we just confirm 200 + 5 rows.
	resp, body = env.do(t, "GET", "/api/products/p1/comments?limit=2000", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	if list := decodeList(t, body); len(list) != 5 {
		t.Fatalf("limit=2000 cap: got %d want 5", len(list))
	}
}

func TestGET_ListComments_InvalidLimit(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "GET", "/api/products/p1/comments?limit=abc", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", resp.StatusCode, body)
	}
}

func TestGET_ListComments_InvalidType(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "GET", "/api/products/p1/comments?type=foo", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	if got := decodeErr(t, body)["code"]; got != "unknown_type" {
		t.Fatalf("code: got %q", got)
	}
}

func TestGET_ListComments_ScopeIsolation(t *testing.T) {
	env := newCommentsTestEnv(t)
	_, _ = env.store.Add(t.Context(), comments.TargetProduct, "X", "a",
		comments.TypeUserNote, "product-X")
	resp, body := env.do(t, "GET", "/api/tasks/X/comments", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	if list := decodeList(t, body); len(list) != 0 {
		t.Fatalf("expected empty list on task/X: got %+v", list)
	}
}

// ---- PATCH ------------------------------------------------------------------

func TestPATCH_EditComment(t *testing.T) {
	env := newCommentsTestEnv(t)
	c, err := env.store.Add(t.Context(), comments.TargetProduct, "p1",
		"alice", comments.TypeUserNote, "original")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp, body := env.do(t, "PATCH", "/api/comments/"+c.ID, editCommentRequest{Body: "edited"})
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d body=%s", resp.StatusCode, body)
	}
	out := decodeComment(t, body)
	if out.Body != "edited" {
		t.Fatalf("body: got %q", out.Body)
	}
	if out.UpdatedAt == nil || out.UpdatedAtISO == "" {
		t.Fatalf("updated_at fields not set: %+v", out)
	}
}

func TestPATCH_NotFound(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "PATCH", "/api/comments/missing-id",
		editCommentRequest{Body: "x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	if got := decodeErr(t, body)["code"]; got != "not_found" {
		t.Fatalf("code: got %q", got)
	}
}

func TestPATCH_EmptyBody(t *testing.T) {
	env := newCommentsTestEnv(t)
	c, _ := env.store.Add(t.Context(), comments.TargetProduct, "p1", "a",
		comments.TypeUserNote, "orig")
	resp, body := env.do(t, "PATCH", "/api/comments/"+c.ID, editCommentRequest{Body: "   "})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	if got := decodeErr(t, body)["code"]; got != "empty_body" {
		t.Fatalf("code: got %q", got)
	}
}

// ---- DELETE -----------------------------------------------------------------

func TestDELETE_Idempotent(t *testing.T) {
	env := newCommentsTestEnv(t)
	c, _ := env.store.Add(t.Context(), comments.TargetProduct, "p1", "a",
		comments.TypeUserNote, "x")
	for i := 0; i < 2; i++ {
		resp, body := env.do(t, "DELETE", "/api/comments/"+c.ID, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("iter %d: got %d body=%s", i, resp.StatusCode, body)
		}
	}
}

// ---- body size --------------------------------------------------------------

func TestPOST_BodyTooLarge(t *testing.T) {
	env := newCommentsTestEnv(t)
	huge := strings.Repeat("a", commentMaxBodyBytes+1024)
	resp, body := env.do(t, "POST", "/api/products/p1/comments",
		[]byte(`{"author":"a","type":"user_note","body":"`+huge+`"}`))
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status: got %d body=%s", resp.StatusCode, body)
	}
	if got := decodeErr(t, body)["code"]; got != "body_too_large" {
		t.Fatalf("code: got %q", got)
	}
}

// ---- body_html (M3-T6) ------------------------------------------------------

func TestPOST_Comment_BodyHTMLParagraph(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "POST", "/api/products/p1/comments", addCommentRequest{
		Author: "alice", Type: string(comments.TypeUserNote), Body: "hello **world**",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d body=%s", resp.StatusCode, body)
	}
	c := decodeComment(t, body)
	if !strings.Contains(c.BodyHTML, "<p>") || !strings.Contains(c.BodyHTML, "<strong>world</strong>") {
		t.Fatalf("body_html missing markdown rendering: %q", c.BodyHTML)
	}
}

func TestPOST_Comment_BodyHTMLCodeBlock(t *testing.T) {
	env := newCommentsTestEnv(t)
	src := "```\nfmt.Println(\"hi\")\n```"
	resp, body := env.do(t, "POST", "/api/products/p1/comments", addCommentRequest{
		Author: "a", Type: string(comments.TypeUserNote), Body: src,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d body=%s", resp.StatusCode, body)
	}
	c := decodeComment(t, body)
	if !strings.Contains(c.BodyHTML, "<pre>") || !strings.Contains(c.BodyHTML, "<code>") {
		t.Fatalf("code block not rendered: %q", c.BodyHTML)
	}
}

func TestPOST_Comment_BodyHTMLGFMTable(t *testing.T) {
	env := newCommentsTestEnv(t)
	src := "| a | b |\n| - | - |\n| 1 | 2 |"
	resp, body := env.do(t, "POST", "/api/products/p1/comments", addCommentRequest{
		Author: "a", Type: string(comments.TypeUserNote), Body: src,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d body=%s", resp.StatusCode, body)
	}
	c := decodeComment(t, body)
	if !strings.Contains(c.BodyHTML, "<table>") {
		t.Fatalf("GFM table not rendered: %q", c.BodyHTML)
	}
}

// TestPOST_Comment_XSSEscape is the acceptance-criteria XSS test: raw
// <script> in body must be escaped in body_html.
func TestPOST_Comment_XSSEscape(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "POST", "/api/products/p1/comments", addCommentRequest{
		Author: "a", Type: string(comments.TypeUserNote),
		Body: "<script>alert(1)</script>",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d body=%s", resp.StatusCode, body)
	}
	c := decodeComment(t, body)
	// goldmark with WithUnsafe() OFF drops raw HTML entirely (replaces with
	// an "<!-- raw HTML omitted -->" comment). The security property we
	// enforce: no executable <script> tag survives into body_html.
	if strings.Contains(c.BodyHTML, "<script>") {
		t.Fatalf("raw <script> tag leaked into body_html: %q", c.BodyHTML)
	}
	if strings.Contains(c.BodyHTML, "alert(1)") {
		t.Fatalf("script payload leaked into body_html: %q", c.BodyHTML)
	}
}

// TestPOST_Comment_EscapedAngleBrackets verifies that when angle brackets
// appear in prose (outside an HTML-looking tag), goldmark escapes them so
// the UI renders them as text rather than HTML.
func TestPOST_Comment_EscapedAngleBrackets(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "POST", "/api/products/p1/comments", addCommentRequest{
		Author: "a", Type: string(comments.TypeUserNote),
		Body: "use the `<T>` type param",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d body=%s", resp.StatusCode, body)
	}
	c := decodeComment(t, body)
	if !strings.Contains(c.BodyHTML, "&lt;T&gt;") {
		t.Fatalf("expected escaped brackets in body_html: %q", c.BodyHTML)
	}
}

// ---- /api/comments/types (M3-T6) --------------------------------------------

func TestGET_CommentsTypes(t *testing.T) {
	env := newCommentsTestEnv(t)
	resp, body := env.do(t, "GET", "/api/comments/types", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d body=%s", resp.StatusCode, body)
	}
	var got []comments.CommentTypeInfo
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, body)
	}
	want := comments.Registry()
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Type != want[i].Type || got[i].Label != want[i].Label {
			t.Fatalf("entry %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

// ---- route wiring -----------------------------------------------------------

// TestRoutes_AllCommentEndpointsRegistered enforces that the registration
// function exposes every URL + method the spec requires.
func TestRoutes_AllCommentEndpointsRegistered(t *testing.T) {
	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()
	registerCommentsRoutes(api, nil)

	want := map[string]string{
		"GET /api/products/{id}/comments":   "",
		"POST /api/products/{id}/comments":  "",
		"GET /api/manifests/{id}/comments":  "",
		"POST /api/manifests/{id}/comments": "",
		"GET /api/tasks/{id}/comments":      "",
		"POST /api/tasks/{id}/comments":     "",
		"PATCH /api/comments/{id}":          "",
		"DELETE /api/comments/{id}":         "",
		"GET /api/comments/types":           "",
	}
	if err := r.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		pathTmpl, err := route.GetPathTemplate()
		if err != nil {
			return nil
		}
		methods, _ := route.GetMethods()
		for _, m := range methods {
			key := m + " " + pathTmpl
			if _, ok := want[key]; ok {
				want[key] = "seen"
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("walk: %v", err)
	}
	for k, v := range want {
		if v != "seen" {
			t.Errorf("route missing: %s", k)
		}
	}
}
