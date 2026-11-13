package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestReorderArgsBackup(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	if err := os.WriteFile("query.sql", []byte("select 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".hidden", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name     string
		in, want []string
	}{
		{"bare --backup is .bak, the file stays a file", []string{"-w", "--backup", "query.sql"}, []string{"-w", "--backup=.bak", "query.sql"}},
		{"bare --backup last", []string{"-w", "query.sql", "--backup"}, []string{"-w", "--backup=.bak", "query.sql"}},
		{"--backup=.ext", []string{"-w", "--backup=.orig", "query.sql"}, []string{"-w", "--backup=.orig", "query.sql"}},
		{"--backup .ext", []string{"-w", "--backup", ".orig", "query.sql"}, []string{"-w", "--backup=.orig", "query.sql"}},
		{"an existing dot file is a file", []string{"-w", "--backup", ".hidden"}, []string{"-w", "--backup=.bak", ".hidden"}},
		{"a path is a file", []string{"-w", "-backup", "./query.sql"}, []string{"-w", "-backup=.bak", "./query.sql"}},
		{"value flags still take the next token", []string{"query.sql", "-c", "cfg.yaml", "--check"}, []string{"-c", "cfg.yaml", "--check", "query.sql"}},
		{"a lone - is stdin, not a flag", []string{"-", "--check"}, []string{"--check", "-"}},
	} {
		if got := reorderArgs(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The diff must be one that patch(1) applies: right line numbers in the hunk headers,
// context before and after, close changes in one hunk, the "no newline" marker.
func TestUnifiedDiff(t *testing.T) {
	lines := func(xs ...string) string { return strings.Join(xs, "\n") + "\n" }
	var a, b []string
	for i := 1; i <= 30; i++ {
		l := "line " + string(rune('A'+i-1))
		a = append(a, l)
		switch i {
		case 10, 12:
			b = append(b, strings.ToUpper(l))
		case 25:
			// deleted
		default:
			b = append(b, l)
		}
	}
	got := unifiedDiff("f.sql", lines(a...), lines(b...))
	want := lines(
		"--- f.sql", "+++ f.sql",
		"@@ -7,9 +7,9 @@",
		" "+a[6], " "+a[7], " "+a[8],
		"-"+a[9], "+"+b[9],
		" "+a[10],
		"-"+a[11], "+"+b[11],
		" "+a[12], " "+a[13], " "+a[14],
		"@@ -22,7 +22,6 @@",
		" "+a[21], " "+a[22], " "+a[23],
		"-"+a[24],
		" "+a[25], " "+a[26], " "+a[27],
	)
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	got = unifiedDiff("f.sql", "select 1;\nSELECT 2", "select 1;\nselect 2;\n")
	want = "--- f.sql\n+++ f.sql\n@@ -1,2 +1,2 @@\n select 1;\n-SELECT 2\n\\ No newline at end of file\n+select 2;\n"
	if got != want {
		t.Errorf("no newline at end: got:\n%s\nwant:\n%s", got, want)
	}

	if got := unifiedDiff("f.sql", "", "a\n"); got != "--- f.sql\n+++ f.sql\n@@ -0,0 +1,1 @@\n+a\n" {
		t.Errorf("empty original: got:\n%s", got)
	}
}

func TestUnifiedDiffAppliesWithPatch(t *testing.T) {
	patch, err := exec.LookPath("patch")
	if err != nil {
		t.Skip("no patch(1)")
	}
	var a, b []string
	for i := 0; i < 200; i++ {
		l := "select " + strings.Repeat("x", i%7) + ";\n"
		a = append(a, l)
		switch {
		case i%37 == 5:
			b = append(b, strings.ToUpper(l))
		case i%53 == 20:
			b = append(b, l, "-- added\n")
		case i%61 == 30:
		default:
			b = append(b, l)
		}
	}
	a[len(a)-1] = "select last" // no newline at the end of the original
	b[len(b)-1] = "select last;\n"
	orig, want := strings.Join(a, ""), strings.Join(b, "")
	dir := t.TempDir()
	file := filepath.Join(dir, "f.sql")
	if err := os.WriteFile(file, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(patch, file)
	cmd.Stdin = strings.NewReader(unifiedDiff("f.sql", orig, want))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("patch: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(file)
	if string(got) != want {
		t.Errorf("patched file differs from the formatted text")
	}
}

// Files of the same name from different directories would overwrite each other below
// --out-dir; a directory argument keeps their paths apart.
func TestOutDirClash(t *testing.T) {
	a, b, dest, clash := outDirClash("out", []input{{"a/x.sql", "x.sql"}, {"b/y.sql", "y.sql"}, {"b/x.sql", "x.sql"}})
	if !clash || a != "a/x.sql" || b != "b/x.sql" || dest != filepath.Join("out", "x.sql") {
		t.Errorf("got %q %q %q %v", a, b, dest, clash)
	}
	if _, _, _, clash := outDirClash("out", []input{{"a/x.sql", filepath.Join("a", "x.sql")}, {"b/x.sql", filepath.Join("b", "x.sql")}}); clash {
		t.Error("different paths below the directory do not clash")
	}
}

func TestUnifiedDiffOfHugeChangeIsCheap(t *testing.T) {
	var a, b strings.Builder
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&a, "select %d;\n", i)
		fmt.Fprintf(&b, "SELECT %d;\n", i)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	d := unifiedDiff("f", a.String(), b.String())
	runtime.ReadMemStats(&after)
	if used := after.TotalAlloc - before.TotalAlloc; used > 1<<30 {
		t.Errorf("diff of two 20000-line files allocated %d MB", used>>20)
	}
	if !strings.HasPrefix(d, "--- f\n+++ f\n@@ -1,20000 +1,20000 @@\n-select 0;\n") {
		t.Errorf("unexpected diff start: %.80q", d)
	}
}
