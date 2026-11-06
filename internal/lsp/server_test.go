package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heptau/pg_procrustes/formatter"
)

// session runs the server on a scripted conversation and returns every message it sent.
func session(t *testing.T, msgs ...any) []map[string]any {
	t.Helper()
	var in bytes.Buffer
	for _, m := range msgs {
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(data), data)
	}
	var out bytes.Buffer
	Serve(&in, &out, Options{Version: "test"})
	var got []map[string]any
	br := bufio.NewReader(&out)
	for {
		body, err := readFrame(br)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatal(err)
		}
		got = append(got, m)
	}
	return got
}

func req(id int, method string, params any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
}

func note(method string, params any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
}

func byID(msgs []map[string]any, id int) map[string]any {
	for _, m := range msgs {
		if v, ok := m["id"].(float64); ok && int(v) == id {
			return m
		}
	}
	return nil
}

func diagnosticsFor(msgs []map[string]any, uri string) []any {
	var last []any
	for _, m := range msgs {
		if m["method"] != "textDocument/publishDiagnostics" {
			continue
		}
		p := m["params"].(map[string]any)
		if p["uri"] == uri {
			last, _ = p["diagnostics"].([]any)
		}
	}
	return last
}

// project makes a directory with a config and returns the URI of a file in it.
func project(t *testing.T, yaml string) (dir, uri string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".pg_procrustes.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, "file://" + filepath.ToSlash(filepath.Join(dir, "a.sql"))
}

func open(uri, text string) map[string]any {
	return note("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "sql", "version": 1, "text": text}})
}

func doc(uri string) map[string]any { return map[string]any{"uri": uri} }

func apply(t *testing.T, src string, edits []any) string {
	t.Helper()
	if len(edits) == 0 {
		return src
	}
	if len(edits) != 1 {
		t.Fatalf("want one edit, got %d", len(edits))
	}
	e := edits[0].(map[string]any)
	r := e["range"].(map[string]any)
	off := func(p map[string]any) int {
		line, ch := int(p["line"].(float64)), int(p["character"].(float64))
		o := 0
		for i := 0; i < line; i++ {
			o += strings.Index(src[o:], "\n") + 1
		}
		return o + ch // ASCII only in these tests
	}
	return src[:off(r["start"].(map[string]any))] + e["newText"].(string) + src[off(r["end"].(map[string]any)):]
}

const keywordsUpper = "reserved_keywords:\n  case: upper\n"

func TestInitializeCapabilities(t *testing.T) {
	got := session(t,
		req(1, "initialize", map[string]any{"rootUri": nil}),
		req(2, "shutdown", nil),
		note("exit", nil),
	)
	res := byID(got, 1)["result"].(map[string]any)
	caps := res["capabilities"].(map[string]any)
	if caps["documentFormattingProvider"] != true || caps["documentRangeFormattingProvider"] != true {
		t.Fatalf("formatting not advertised: %v", caps)
	}
	if res["serverInfo"].(map[string]any)["version"] != "test" {
		t.Fatalf("serverInfo: %v", res["serverInfo"])
	}
	if r, ok := byID(got, 2)["result"]; !ok || r != nil {
		t.Fatalf("shutdown must reply result null, got %v", byID(got, 2))
	}
}

func TestRequestBeforeInitialize(t *testing.T) {
	got := session(t, req(1, "textDocument/formatting", map[string]any{"textDocument": doc("file:///x.sql")}))
	if byID(got, 1)["error"] == nil {
		t.Fatalf("want an error before initialize, got %v", got)
	}
}

func TestUnknownRequestAndNotification(t *testing.T) {
	got := session(t,
		req(1, "initialize", map[string]any{}),
		req(2, "textDocument/hover", map[string]any{}),
		note("$/somethingElse", nil),
	)
	if byID(got, 2)["error"] == nil {
		t.Fatalf("unknown request must fail with MethodNotFound: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("unknown notification must be ignored, got %d messages", len(got))
	}
}

func TestFormatDocument(t *testing.T) {
	_, uri := project(t, "reserved_keywords:\n  case: upper\n")
	src := "select a from t;\n"
	got := session(t,
		req(1, "initialize", map[string]any{}),
		open(uri, src),
		req(2, "textDocument/formatting", map[string]any{"textDocument": doc(uri), "options": map[string]any{"tabSize": 2}}),
	)
	edits := byID(got, 2)["result"].([]any)
	out := apply(t, src, edits)
	if out != "SELECT a FROM t;\n" {
		t.Fatalf("got %q", out)
	}
}

func TestFormatIsMinimalEdit(t *testing.T) {
	_, uri := project(t, keywordsUpper)
	src := "SELECT 1;\n\nselect 2;\n\nSELECT 3;\n"
	got := session(t,
		req(1, "initialize", map[string]any{}),
		open(uri, src),
		req(2, "textDocument/formatting", map[string]any{"textDocument": doc(uri)}),
	)
	edits := byID(got, 2)["result"].([]any)
	if len(edits) != 1 {
		t.Fatalf("want 1 edit, got %v", edits)
	}
	r := edits[0].(map[string]any)["range"].(map[string]any)
	if r["start"].(map[string]any)["line"].(float64) != 2 || r["end"].(map[string]any)["line"].(float64) != 3 {
		t.Fatalf("edit should cover only line 2, got %v", r)
	}
	if apply(t, src, edits) != "SELECT 1;\n\nSELECT 2;\n\nSELECT 3;\n" {
		t.Fatalf("wrong result")
	}
}

func TestFormatAlreadyFormattedGivesNoEdits(t *testing.T) {
	_, uri := project(t, keywordsUpper)
	got := session(t,
		req(1, "initialize", map[string]any{}),
		open(uri, "SELECT 1;\n"),
		req(2, "textDocument/formatting", map[string]any{"textDocument": doc(uri)}),
	)
	if edits := byID(got, 2)["result"].([]any); len(edits) != 0 {
		t.Fatalf("want no edits, got %v", edits)
	}
}

func TestFormatRange(t *testing.T) {
	_, uri := project(t, keywordsUpper)
	src := "select 1;\nselect 2;\nselect 3;\n"
	got := session(t,
		req(1, "initialize", map[string]any{}),
		open(uri, src),
		req(2, "textDocument/rangeFormatting", map[string]any{
			"textDocument": doc(uri),
			"range": map[string]any{
				"start": map[string]any{"line": 1, "character": 0},
				"end":   map[string]any{"line": 2, "character": 0}, // ends at column 0: line 3 is not included
			},
		}),
	)
	out := apply(t, src, byID(got, 2)["result"].([]any))
	if out != "select 1;\nSELECT 2;\nselect 3;\n" {
		t.Fatalf("got %q", out)
	}
}

func TestFormatUsesEditorBufferNotDisk(t *testing.T) {
	_, uri := project(t, keywordsUpper)
	got := session(t,
		req(1, "initialize", map[string]any{}),
		open(uri, "select 1;\n"),
		note("textDocument/didChange", map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": 2},
			"contentChanges": []any{map[string]any{"text": "select 2;\n"}},
		}),
		req(2, "textDocument/formatting", map[string]any{"textDocument": doc(uri)}),
	)
	if out := apply(t, "select 2;\n", byID(got, 2)["result"].([]any)); out != "SELECT 2;\n" {
		t.Fatalf("got %q", out)
	}
}

func TestSyntaxErrorDiagnosticAndNoEdit(t *testing.T) {
	_, uri := project(t, keywordsUpper)
	src := "select 1;\nselect from from;\n"
	got := session(t,
		req(1, "initialize", map[string]any{}),
		open(uri, src),
		req(2, "textDocument/formatting", map[string]any{"textDocument": doc(uri)}),
	)
	diags := diagnosticsFor(got, uri)
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %v", diags)
	}
	d := diags[0].(map[string]any)
	if d["range"].(map[string]any)["start"].(map[string]any)["line"].(float64) != 1 {
		t.Fatalf("diagnostic on wrong line: %v", d)
	}
	if d["severity"].(float64) != severityError || d["source"] != "pg_procrustes" {
		t.Fatalf("bad diagnostic: %v", d)
	}
	if res := byID(got, 2); res["error"] != nil || len(res["result"].([]any)) != 0 {
		t.Fatalf("broken document must not be edited and must not raise an error: %v", res)
	}
}

func TestSkipModeFormatsValidStatements(t *testing.T) {
	_, uri := project(t, "syntax_errors: skip\nreserved_keywords:\n  case: upper\n")
	src := "select 1;\nselect from from;\n"
	got := session(t,
		req(1, "initialize", map[string]any{}),
		open(uri, src),
		req(2, "textDocument/formatting", map[string]any{"textDocument": doc(uri)}),
	)
	if out := apply(t, src, byID(got, 2)["result"].([]any)); out != "SELECT 1;\nselect from from;\n" {
		t.Fatalf("got %q", out)
	}
	diags := diagnosticsFor(got, uri)
	if len(diags) != 1 || diags[0].(map[string]any)["severity"].(float64) != severityWarning {
		t.Fatalf("skip mode reports a warning: %v", diags)
	}
}

func TestDiagnosticsClearedWhenFixedAndOnClose(t *testing.T) {
	_, uri := project(t, keywordsUpper)
	got := session(t,
		req(1, "initialize", map[string]any{}),
		open(uri, "select from from;\n"),
		req(2, "workspace/executeCommand", nil), // a request in between flushes the queue
		note("textDocument/didChange", map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": 2},
			"contentChanges": []any{map[string]any{"text": "select 1;\n"}},
		}),
		req(3, "workspace/executeCommand", nil),
	)
	if d := diagnosticsFor(got, uri); len(d) != 0 {
		t.Fatalf("diagnostics must be cleared after the fix, got %v", d)
	}
	got = session(t,
		req(1, "initialize", map[string]any{}),
		open(uri, "select from from;\n"),
		note("textDocument/didClose", map[string]any{"textDocument": doc(uri)}),
	)
	if d := diagnosticsFor(got, uri); d == nil || len(d) != 0 {
		t.Fatalf("closing publishes an empty list, got %v", d)
	}
}

func TestBadConfigIsAnErrorResponse(t *testing.T) {
	_, uri := project(t, "reserved_keywords:\n  case: sideways\n")
	got := session(t,
		req(1, "initialize", map[string]any{}),
		open(uri, "select 1;\n"),
		req(2, "textDocument/formatting", map[string]any{"textDocument": doc(uri)}),
	)
	if byID(got, 2)["error"] == nil {
		t.Fatalf("an invalid config must be reported: %v", byID(got, 2))
	}
}

func TestConfigIsReReadOnEveryRequest(t *testing.T) {
	dir, uri := project(t, keywordsUpper)
	var in bytes.Buffer
	send := func(m any) {
		data, _ := json.Marshal(m)
		fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(data), data)
	}
	send(req(1, "initialize", map[string]any{}))
	send(open(uri, "select 1;\n"))
	send(req(2, "textDocument/formatting", map[string]any{"textDocument": doc(uri)}))
	var out bytes.Buffer
	Serve(&in, &out, Options{})
	if !strings.Contains(out.String(), "SELECT 1;") {
		t.Fatalf("first run should upper-case: %s", out.String())
	}
	if err := os.WriteFile(filepath.Join(dir, ".pg_procrustes.yaml"), []byte("reserved_keywords:\n  case: lower\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in.Reset()
	out.Reset()
	send(req(1, "initialize", map[string]any{}))
	send(open(uri, "SELECT 1;\n"))
	send(req(2, "textDocument/formatting", map[string]any{"textDocument": doc(uri)}))
	Serve(&in, &out, Options{})
	if !strings.Contains(out.String(), "select 1;") {
		t.Fatalf("changed config should apply: %s", out.String())
	}
}

func TestUTF16Positions(t *testing.T) {
	// 😀 is one rune but two UTF-16 code units; é is one of each.
	text := "-- 😀é\nselect x"
	if p := offsetToPosition(text, strings.Index(text, "é")+len("é")); p != (position{0, 6}) {
		t.Fatalf("got %+v", p)
	}
	if p := offsetToPosition(text, len(text)); p != (position{1, 8}) {
		t.Fatalf("got %+v", p)
	}
	r := errorRange([]string{"-- 😀 select"}, syntaxErr(1, 6))
	if r.Start.Character != 6 || r.End.Character != 12 {
		t.Fatalf("got %+v", r)
	}
}

func TestMinimalEditsEdgeCases(t *testing.T) {
	for _, c := range [][2]string{
		{"a", "b"}, {"a\n", "a\nb\n"}, {"a\nb\n", "a\n"}, {"", "x\n"}, {"a\nb", "a\nc"},
		{"a\r\nb\r\n", "a\r\nc\r\n"}, {"x\n\n\n", "x\n"},
	} {
		edits := minimalEdits(c[0], c[1])
		raw, _ := json.Marshal(edits)
		var generic []any
		_ = json.Unmarshal(raw, &generic)
		if got := apply(t, c[0], generic); got != c[1] {
			t.Errorf("%q → %q: got %q", c[0], c[1], got)
		}
	}
}

func TestURIToPath(t *testing.T) {
	if got := uriToPath("file:///tmp/a%20b.sql"); got != filepath.FromSlash("/tmp/a b.sql") {
		t.Fatalf("got %q", got)
	}
	if got := uriToPath("untitled:Untitled-1"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestBadFramingAndJSON(t *testing.T) {
	var in bytes.Buffer
	body := "{not json"
	fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(body), body)
	var out bytes.Buffer
	if code := Serve(&in, &out, Options{}); code != 1 {
		t.Fatalf("input ending without shutdown must exit 1, got %d", code)
	}
	if !strings.Contains(out.String(), "-32700") {
		t.Fatalf("want a parse error, got %s", out.String())
	}
}

func TestExitCodes(t *testing.T) {
	run := func(msgs ...any) int {
		var in bytes.Buffer
		for _, m := range msgs {
			data, _ := json.Marshal(m)
			fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(data), data)
		}
		return Serve(&in, io.Discard, Options{})
	}
	if c := run(req(1, "initialize", map[string]any{}), req(2, "shutdown", nil), note("exit", nil)); c != 0 {
		t.Fatalf("clean exit: %d", c)
	}
	if c := run(req(1, "initialize", map[string]any{}), note("exit", nil)); c != 1 {
		t.Fatalf("exit without shutdown: %d", c)
	}
}

func syntaxErr(line, col int) formatter.SyntaxError {
	return formatter.SyntaxError{Line: line, Col: col}
}
