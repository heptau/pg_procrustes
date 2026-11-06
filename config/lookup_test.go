package config

import (
	"os"
	"path/filepath"
	"testing"
)

// The config is found from the file's directory, whatever the working directory is.
func TestConfigPathForUsesTheFilesDirectory(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	deep := filepath.Join(proj, "sql", "sub")
	other := filepath.Join(root, "other")
	for _, d := range []string{deep, other, filepath.Join(root, "elsewhere")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	projCfg := filepath.Join(proj, ".pg_procrustes.yaml")
	otherCfg := filepath.Join(other, ".pg_procrustes.yaml")
	for _, f := range []string{projCfg, otherCfg} {
		if err := os.WriteFile(f, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(wd) })
	if err := os.Chdir(other); err != nil { // cwd has its own config, the file's project must still win
		t.Fatal(err)
	}
	if got := ConfigPathFor(filepath.Join(deep, "a.sql")); got != projCfg {
		t.Errorf("deep file: got %q, want %q", got, projCfg)
	}
	// The file need not exist (stdin with --stdin-filepath).
	if got := ConfigPathFor(filepath.Join(proj, "not", "yet.sql")); got != projCfg {
		t.Errorf("missing file: got %q, want %q", got, projCfg)
	}
	if got := ConfigPathFor(filepath.Join(other, "b.sql")); got != otherCfg {
		t.Errorf("other project: got %q, want %q", got, otherCfg)
	}
	// Nothing above the file: fall back to the working directory's config.
	got, _ := filepath.EvalSymlinks(ConfigPathFor(filepath.Join(root, "elsewhere", "c.sql")))
	want, _ := filepath.EvalSymlinks(otherCfg)
	if got != want { // (the working directory is reported with symlinks resolved on macOS)
		t.Errorf("fallback: got %q, want %q", got, want)
	}
}
