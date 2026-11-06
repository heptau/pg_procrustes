package config

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTemplatesURL(t *testing.T) {
	if got := TemplatesURL("0.3.0"); got != "https://github.com/heptau/pg_procrustes/releases/download/v0.3.0/templates.zip" {
		t.Errorf("release: %s", got)
	}
	if got := TemplatesURL("v1.2.3"); !strings.Contains(got, "/download/v1.2.3/") {
		t.Errorf("v prefix: %s", got)
	}
	for _, v := range []string{"dev", "", "0.3.0-rc1", "v0.0.0-2024-abc"} {
		if got := TemplatesURL(v); !strings.Contains(got, "/latest/download/") {
			t.Errorf("%q should use latest: %s", v, got)
		}
	}
}

func TestInstallTemplatesFromZipFile(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "t.zip")
	data := makeZip(t, map[string]string{
		"ai.yaml":                 "keywords:\n  case: lower\n",
		"templates/modern.yaml":   "keywords:\n  case: upper\n",
		"../evil.yaml":            "x: 1\n",
		"sub/dir/nested.yaml":     "x: 1\n",
		"templates/bad name.yaml": "x: 1\n",
		"README.md":               "hi",
	})
	if err := os.WriteFile(zipPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out")
	got, err := InstallTemplates(zipPath, dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "ai" || got[1].Name != "modern" {
		t.Fatalf("installed %+v", got)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 2 {
		t.Errorf("unexpected files in %s: %v", dest, entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.yaml")); err == nil {
		t.Error("path traversal wrote outside dest")
	}
	// a second run replaces and says so
	again, err := InstallTemplates(zipPath, dest)
	if err != nil || !again[0].Replaced {
		t.Errorf("second run: %+v %v", again, err)
	}
}

func TestInstallTemplatesFromDirectory(t *testing.T) {
	src := t.TempDir()
	_ = os.WriteFile(filepath.Join(src, "a.yaml"), []byte("keywords:\n  case: lower\n"), 0o644)
	_ = os.WriteFile(filepath.Join(src, "notes.txt"), []byte("x"), 0o644)
	dest := filepath.Join(t.TempDir(), "d")
	got, err := InstallTemplates(src, dest)
	if err != nil || len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestInstallTemplatesOverHTTP(t *testing.T) {
	data := makeZip(t, map[string]string{"x.yaml": "keywords:\n  case: lower\n"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/templates.zip" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	defer srv.Close()
	dest := t.TempDir()
	got, err := InstallTemplates(srv.URL+"/templates.zip", dest)
	if err != nil || len(got) != 1 || got[0].Name != "x" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := InstallTemplates(srv.URL+"/missing.zip", dest); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("404 expected, got %v", err)
	}
}

func TestInstallTemplatesRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	cases := map[string][]byte{
		"notzip.zip":   []byte("hello"),
		"empty.zip":    makeZip(t, map[string]string{"README.md": "x"}),
		"notyaml.zip":  makeZip(t, map[string]string{"a.yaml": "- just\n- a list\n"}),
		"toolarge.zip": makeZip(t, map[string]string{"a.yaml": "k: " + strings.Repeat("x", maxTemplateFile+10) + "\n"}),
	}
	for name, data := range cases {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, data, 0o644)
		dest := filepath.Join(dir, "dest-"+name)
		if _, err := InstallTemplates(p, dest); err == nil {
			t.Errorf("%s: expected an error", name)
		}
		if _, err := os.Stat(dest); err == nil {
			t.Errorf("%s: dest created despite the error", name)
		}
	}
}

func TestUserTemplateDirHonoursEnv(t *testing.T) {
	t.Setenv("PG_PROCRUSTES_TEMPLATES", "/x/one"+string(os.PathListSeparator)+"/x/two")
	if d, _ := UserTemplateDir(); d != "/x/one" {
		t.Errorf("got %s", d)
	}
}
