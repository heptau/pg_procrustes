// Package lsp implements a small Language Server Protocol server over stdio for
// pg_procrustes: whole-document and range formatting plus syntax-error diagnostics.
// It speaks JSON-RPC 2.0 with Content-Length framing and uses only the standard library.
//
// The configuration is looked up per document exactly as the command line does
// (nearest .pg_procrustes.yaml above the file) and is re-read on every request, so
// edits to the config take effect without restarting the server.
package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

// Options configure Serve.
type Options struct {
	ConfigPath string    // fixed config file (-c); "" = nearest .pg_procrustes.yaml per document
	Version    string    // reported in serverInfo
	Log        io.Writer // diagnostics about the server itself (stderr); nil = discard
}

// JSON-RPC / LSP error codes.
const (
	codeParseError       = -32700
	codeInvalidRequest   = -32600
	codeMethodNotFound   = -32601
	codeInvalidParams    = -32602
	codeInternalError    = -32603
	codeServerNotInit    = -32002
	codeRequestFailed    = -32803
	severityError        = 1
	severityWarning      = 2
	textDocumentSyncFull = 1
)

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type textEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"newText"`
}

type diagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"`
	Source   string   `json:"source"`
	Message  string   `json:"message"`
}

type document struct {
	text    string
	version int
}

type server struct {
	opts        Options
	out         io.Writer
	outMu       sync.Mutex
	docs        map[string]*document
	dirty       map[string]bool
	root        string
	initConfig  string
	initialized bool
	shutdown    bool
	exited      bool
	exitCode    int
}

// Serve runs the server until the client sends exit or the input ends and returns the
// process exit code: 0 after a clean shutdown/exit sequence, 1 otherwise.
func Serve(in io.Reader, out io.Writer, opts Options) int {
	if opts.Log == nil {
		opts.Log = io.Discard
	}
	s := &server{opts: opts, out: out, docs: map[string]*document{}, dirty: map[string]bool{}}

	raw := make(chan []byte, 64)
	go func() {
		defer close(raw)
		br := bufio.NewReader(in)
		for {
			body, err := readFrame(br)
			if err != nil {
				if err != io.EOF {
					_, _ = fmt.Fprintf(opts.Log, "pg_procrustes lsp: read: %v\n", err)
				}
				return
			}
			raw <- body
		}
	}()

	for body := range raw {
		var m message
		if err := json.Unmarshal(body, &m); err != nil {
			s.replyError(json.RawMessage("null"), codeParseError, "parse error: "+err.Error())
			continue
		}
		s.handle(&m)
		if s.exited {
			return s.exitCode
		}
		// While more input is queued (typing fast), diagnostics wait: only the last one counts.
		if len(raw) == 0 {
			s.flushDiagnostics()
		}
	}
	if s.shutdown {
		return 0
	}
	return 1
}

// readFrame reads one Content-Length framed message body.
func readFrame(br *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF && line == "" && length < 0 {
				return nil, io.EOF
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if length < 0 {
				return nil, errors.New("message without Content-Length header")
			}
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 0 {
				return nil, fmt.Errorf("bad Content-Length %q", v)
			}
			length = n
		}
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(br, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (s *server) send(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		_, _ = fmt.Fprintf(s.opts.Log, "pg_procrustes lsp: marshal: %v\n", err)
		return
	}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	_, _ = fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(data))
	_, _ = s.out.Write(data)
}

func (s *server) reply(id json.RawMessage, result any) {
	s.send(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{"2.0", id, result})
}

func (s *server) replyError(id json.RawMessage, code int, msg string) {
	type rpcError struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	s.send(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   rpcError        `json:"error"`
	}{"2.0", id, rpcError{code, msg}})
}

func (s *server) notify(method string, params any) {
	s.send(struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", method, params})
}

func (s *server) showMessage(typ int, text string) {
	s.notify("window/showMessage", map[string]any{"type": typ, "message": text})
}

func (s *server) handle(m *message) {
	isRequest := m.ID != nil && m.Method != ""

	if !s.initialized && m.Method != "initialize" && m.Method != "exit" {
		if isRequest {
			s.replyError(m.ID, codeServerNotInit, "server not initialized")
		}
		return
	}
	if s.shutdown && m.Method != "exit" {
		if isRequest {
			s.replyError(m.ID, codeInvalidRequest, "server is shutting down")
		}
		return
	}

	switch m.Method {
	case "initialize":
		s.initialize(m)
	case "shutdown":
		s.shutdown = true
		s.reply(m.ID, nil)
	case "exit":
		s.exited = true
		if s.shutdown {
			s.exitCode = 0
		} else {
			s.exitCode = 1
		}
	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
				Text    string `json:"text"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(m.Params, &p) == nil {
			s.docs[p.TextDocument.URI] = &document{text: p.TextDocument.Text, version: p.TextDocument.Version}
			s.dirty[p.TextDocument.URI] = true
		}
	case "textDocument/didChange":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if json.Unmarshal(m.Params, &p) == nil && len(p.ContentChanges) > 0 {
			// Full sync: the last change carries the whole text.
			s.docs[p.TextDocument.URI] = &document{
				text:    p.ContentChanges[len(p.ContentChanges)-1].Text,
				version: p.TextDocument.Version,
			}
			s.dirty[p.TextDocument.URI] = true
		}
	case "textDocument/didSave":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(m.Params, &p) == nil {
			if _, ok := s.docs[p.TextDocument.URI]; ok {
				s.dirty[p.TextDocument.URI] = true // the config may have changed on disk
			}
		}
	case "textDocument/didClose":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(m.Params, &p) == nil {
			delete(s.docs, p.TextDocument.URI)
			delete(s.dirty, p.TextDocument.URI)
			s.notify("textDocument/publishDiagnostics", map[string]any{"uri": p.TextDocument.URI, "diagnostics": []diagnostic{}})
		}
	case "workspace/didChangeWatchedFiles", "workspace/didChangeConfiguration":
		for uri := range s.docs {
			s.dirty[uri] = true
		}
	case "textDocument/formatting", "textDocument/rangeFormatting":
		s.format(m)
	default:
		if isRequest {
			s.replyError(m.ID, codeMethodNotFound, "method not supported: "+m.Method)
		}
	}
}

func (s *server) initialize(m *message) {
	var p struct {
		RootURI               string `json:"rootUri"`
		RootPath              string `json:"rootPath"`
		InitializationOptions struct {
			ConfigPath string `json:"configPath"`
		} `json:"initializationOptions"`
		WorkspaceFolders []struct {
			URI string `json:"uri"`
		} `json:"workspaceFolders"`
	}
	_ = json.Unmarshal(m.Params, &p)
	switch {
	case p.RootURI != "":
		s.root = uriToPath(p.RootURI)
	case len(p.WorkspaceFolders) > 0:
		s.root = uriToPath(p.WorkspaceFolders[0].URI)
	case p.RootPath != "":
		s.root = p.RootPath
	}
	s.initConfig = p.InitializationOptions.ConfigPath
	s.initialized = true

	version := s.opts.Version
	if version == "" {
		version = "dev"
	}
	s.reply(m.ID, map[string]any{
		"capabilities": map[string]any{
			"positionEncoding": "utf-16",
			"textDocumentSync": map[string]any{
				"openClose": true,
				"change":    textDocumentSyncFull,
				"save":      map[string]any{"includeText": false},
			},
			"documentFormattingProvider":      true,
			"documentRangeFormattingProvider": true,
		},
		"serverInfo": map[string]any{"name": "pg_procrustes", "version": version},
	})
}

// loadConfig finds and loads the configuration for the document uri.
func (s *server) loadConfig(uri string) (*config.Config, error) {
	path := s.opts.ConfigPath
	if path == "" {
		path = s.initConfig
	}
	if s.opts.ConfigPath == "" && path != "" && !filepath.IsAbs(path) && s.root != "" {
		// configPath from the editor's settings ("db/.pg_procrustes.yaml") is relative to the
		// workspace, not to the directory the editor happened to start the server in.
		path = filepath.Join(s.root, path)
	}
	if path != "" {
		// A config file that was named must exist (a typo must not silently mean "no config").
		if _, err := os.Stat(path); err != nil {
			return nil, err
		}
	}
	if path == "" {
		file := uriToPath(uri)
		if file == "" && s.root != "" {
			file = filepath.Join(s.root, "untitled.sql")
		}
		if file != "" {
			path = config.ConfigPathFor(file)
		}
	}
	return config.Load(path)
}

// text returns the buffer the editor has open, else the file on disk.
func (s *server) text(uri string) (string, bool) {
	if d, ok := s.docs[uri]; ok {
		return d.text, true
	}
	if p := uriToPath(uri); p != "" {
		if data, err := os.ReadFile(p); err == nil {
			return string(data), true
		}
	}
	return "", false
}

func (s *server) format(m *message) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Range *lspRange `json:"range"`
	}
	if err := json.Unmarshal(m.Params, &p); err != nil {
		s.replyError(m.ID, codeInvalidParams, err.Error())
		return
	}
	uri := p.TextDocument.URI
	src, ok := s.text(uri)
	if !ok {
		s.replyError(m.ID, codeInvalidParams, "document is not open: "+uri)
		return
	}
	cfg, err := s.loadConfig(uri)
	if err != nil {
		s.replyError(m.ID, codeRequestFailed, "pg_procrustes config: "+err.Error())
		return
	}

	var (
		out   string
		diags []formatter.SyntaxError
	)
	if m.Method == "textDocument/rangeFormatting" && p.Range != nil {
		start, end := lineRange(src, p.Range)
		out, diags, err = formatter.FormatRange(src, cfg, start, end)
	} else {
		out, diags, err = formatter.FormatWithDiagnostics(src, cfg)
	}

	var fatal formatter.SyntaxErrors
	switch {
	case errors.As(err, &fatal):
		diags = fatal
		out = ""
	case err != nil:
		s.replyError(m.ID, codeRequestFailed, "pg_procrustes: "+err.Error())
		return
	}
	// Syntax errors are shown as diagnostics; a document that does not parse is not touched.
	// An empty result for a non-empty document is never an edit that wipes the buffer.
	if (len(diags) > 0 && cfg.SyntaxErrors != config.SyntaxErrorSkip && cfg.SyntaxErrors != config.SyntaxErrorIgnore) ||
		(out == "" && strings.TrimSpace(src) != "") {
		s.reply(m.ID, []textEdit{})
		return
	}
	s.reply(m.ID, minimalEdits(src, out))
}

// lineRange converts an LSP range to the 1-based inclusive line range FormatRange wants.
// A range that ends at column 0 of a later line does not include that line.
func lineRange(src string, r *lspRange) (int, int) {
	start := r.Start.Line + 1
	end := r.End.Line + 1
	if r.End.Character == 0 && r.End.Line > r.Start.Line {
		end--
	}
	if end < start {
		end = start
	}
	return start, end
}

// minimalEdits returns the edit that turns src into out: the lines they differ in, so the
// editor keeps cursor, folds and undo granularity for the rest of the document.
func minimalEdits(src, out string) []textEdit {
	if src == out {
		return []textEdit{}
	}
	a := strings.SplitAfter(src, "\n")
	b := strings.SplitAfter(out, "\n")
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	startOff := 0
	for _, l := range a[:p] {
		startOff += len(l)
	}
	endOff := len(src)
	for _, l := range a[len(a)-s:] {
		endOff -= len(l)
	}
	return []textEdit{{
		Range:   lspRange{Start: offsetToPosition(src, startOff), End: offsetToPosition(src, endOff)},
		NewText: strings.Join(b[p:len(b)-s], ""),
	}}
}

// offsetToPosition converts a byte offset to an LSP position (UTF-16 columns).
func offsetToPosition(text string, off int) position {
	if off > len(text) {
		off = len(text)
	}
	line := strings.Count(text[:off], "\n")
	lineStart := strings.LastIndex(text[:off], "\n") + 1
	return position{Line: line, Character: utf16Len(text[lineStart:off])}
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += len(utf16.Encode([]rune{r}))
	}
	return n
}

// flushDiagnostics publishes the syntax errors of every document that changed.
func (s *server) flushDiagnostics() {
	if len(s.dirty) == 0 {
		return
	}
	uris := make([]string, 0, len(s.dirty))
	for uri := range s.dirty {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	s.dirty = map[string]bool{}
	for _, uri := range uris {
		d, ok := s.docs[uri]
		if !ok {
			continue
		}
		s.notify("textDocument/publishDiagnostics", map[string]any{
			"uri":         uri,
			"version":     d.version,
			"diagnostics": s.diagnose(uri, d.text),
		})
	}
}

func (s *server) diagnose(uri, text string) []diagnostic {
	list := []diagnostic{}
	cfg, err := s.loadConfig(uri)
	if err != nil {
		s.showMessage(1, "pg_procrustes config: "+err.Error())
		return list
	}
	_, diags, err := formatter.FormatWithDiagnostics(text, cfg)
	var fatal formatter.SyntaxErrors
	if errors.As(err, &fatal) {
		diags = fatal
	}
	sev := severityError
	if cfg.SyntaxErrors == config.SyntaxErrorSkip {
		sev = severityWarning
	}
	lines := strings.Split(text, "\n")
	for _, e := range diags {
		if e.Line == 1 && strings.HasPrefix(text, "\uFEFF") {
			e.Col++ // the byte order mark is a character of the first line for the editor
		}
		list = append(list, diagnostic{
			Range:    errorRange(lines, e),
			Severity: sev,
			Source:   "pg_procrustes",
			Message:  e.Message,
		})
	}
	return list
}

// errorRange turns the 1-based line/character position of a syntax error into a range
// covering the word at that position (at least one character).
func errorRange(lines []string, e formatter.SyntaxError) lspRange {
	line := e.Line - 1
	if line < 0 {
		line = 0
	}
	if line >= len(lines) {
		line = len(lines) - 1
	}
	text := strings.TrimRight(lines[line], "\r")
	runes := []rune(text)
	col := e.Col - 1
	if col < 0 {
		col = 0
	}
	if col > len(runes) {
		col = len(runes)
	}
	end := col
	for end < len(runes) && isWordRune(runes[end]) {
		end++
	}
	if end == col && end < len(runes) {
		end++
	}
	return lspRange{
		Start: position{Line: line, Character: utf16Len(string(runes[:col]))},
		End:   position{Line: line, Character: utf16Len(string(runes[:end]))},
	}
}

func isWordRune(r rune) bool {
	return r == '_' || r == '$' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= utf8.RuneSelf
}

// uriToPath converts a file: URI to a local path; other schemes give "".
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	p := u.Path
	if runtime.GOOS == "windows" && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}
