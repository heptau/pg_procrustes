package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeTree(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("select 1;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func paths(in []input, root string) []string {
	var out []string
	for _, i := range in {
		r := i.path
		if filepath.IsAbs(r) {
			r, _ = filepath.Rel(root, r)
		}
		out = append(out, filepath.ToSlash(r))
	}
	return out
}

func TestExpandInputs(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "top.sql", "a/one.sql", "a/b/two.SQL", "a/readme.txt", "node_modules/x/n.sql", ".git/h.sql", "a/.hidden/h.sql", "a/b/c/three.sql")
	chdir := func(dir string) {
		wd, _ := os.Getwd()
		t.Cleanup(func() { _ = os.Chdir(wd) })
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
	}
	chdir(root)

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"directory is searched recursively", []string{"a"}, []string{"a/b/c/three.sql", "a/b/two.SQL", "a/one.sql"}},
		{"dot: hidden dirs and node_modules are skipped", []string{"."}, []string{"a/b/c/three.sql", "a/b/two.SQL", "a/one.sql", "top.sql"}},
		{"** matches any depth, also none", []string{"**/*.sql"}, []string{"a/b/c/three.sql", "a/one.sql", "top.sql"}},
		{"** with a fixed prefix", []string{"a/**/*.sql"}, []string{"a/b/c/three.sql", "a/one.sql"}},
		{"** in the middle", []string{"a/**/three.sql"}, []string{"a/b/c/three.sql"}},
		{"plain glob stays one level", []string{"a/*.sql"}, []string{"a/one.sql"}},
		{"a glob that matches a directory searches it", []string{"a/b*"}, []string{"a/b/c/three.sql", "a/b/two.SQL"}},
		{"named files are kept whatever the extension", []string{"a/readme.txt"}, []string{"a/readme.txt"}},
		{"no match is kept so the open error shows", []string{"missing/*.sql"}, []string{"missing/*.sql"}},
		{"no duplicates", []string{"a", "a/one.sql", "a/*.sql"}, []string{"a/b/c/three.sql", "a/b/two.SQL", "a/one.sql"}},
	}
	for _, c := range cases {
		got, err := expandInputs(c.args)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if p := paths(got, root); !reflect.DeepEqual(p, c.want) {
			t.Errorf("%s:\n got %v\nwant %v", c.name, p, c.want)
		}
	}
}

func TestExpandInputsExistingFileWithPatternCharacters(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "v[1].sql", "v1.sql")
	got, err := expandInputs([]string{filepath.Join(root, "v[1].sql")})
	if err != nil {
		t.Fatal(err)
	}
	if p := paths(got, root); !reflect.DeepEqual(p, []string{"v[1].sql"}) {
		t.Errorf("got %v, want only v[1].sql", p)
	}
}

func TestExpandInputsRelativeOutputPaths(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "sql/a/one.sql", "sql/top.sql")
	got, err := expandInputs([]string{filepath.Join(root, "sql")})
	if err != nil {
		t.Fatal(err)
	}
	var rels []string
	for _, i := range got {
		rels = append(rels, filepath.ToSlash(i.rel))
	}
	if want := []string{"a/one.sql", "top.sql"}; !reflect.DeepEqual(rels, want) {
		t.Errorf("got %v want %v", rels, want)
	}
}

func TestMatchSegments(t *testing.T) {
	cases := []struct {
		pat, name string
		want      bool
	}{
		{"**/*.sql", "a.sql", true},
		{"**/*.sql", "x/y/a.sql", true},
		{"**/*.sql", "x/y/a.txt", false},
		{"a/**", "a/b/c", true},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/*/b", "a/x/y/b", false},
	}
	for _, c := range cases {
		if got := matchSegments(splitSlash(c.pat), splitSlash(c.name)); got != c.want {
			t.Errorf("%q ~ %q: got %v", c.pat, c.name, got)
		}
	}
}

func splitSlash(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '/' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}
