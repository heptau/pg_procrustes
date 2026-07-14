package formatter_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

// Run with -update to rewrite want.sql files instead of comparing.
//
//	go test ./formatter/... -run TestGolden -update
var goldenUpdate = flag.Bool("update", false, "rewrite want.sql golden files")

// TestGolden runs every subdirectory of testdata/ as a formatting golden test.
// Each case directory must contain:
//
//	input.sql   — SQL source to format
//	want.sql    — expected formatter output (generated/updated via -update)
//	config.yaml — optional; partial config merged on top of defaults
func TestGolden(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if os.IsNotExist(err) {
		t.Skip("no testdata directory")
	}
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join("testdata", name)

			src, err := os.ReadFile(filepath.Join(dir, "input.sql"))
			if err != nil {
				t.Fatalf("input.sql: %v", err)
			}

			// config.Load accepts an explicit path; returns defaultConfig() when
			// the file does not exist, so a missing config.yaml is fine.
			cfg, err := config.Load(filepath.Join(dir, "config.yaml"))
			if err != nil {
				t.Fatalf("config.yaml: %v", err)
			}

			got, err := formatter.Format(string(src), cfg)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}

			// Idempotence check: formatting the output again must be a no-op.
			got2, err := formatter.Format(got, cfg)
			if err != nil {
				t.Fatalf("Format (idempotence pass): %v", err)
			}
			if got2 != got {
				t.Errorf("formatter is not idempotent for %s", name)
			}

			wantPath := filepath.Join(dir, "want.sql")
			if *goldenUpdate {
				if err := os.WriteFile(wantPath, []byte(got), 0o644); err != nil {
					t.Fatalf("write want.sql: %v", err)
				}
				t.Logf("updated %s", wantPath)
				return
			}

			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("want.sql: %v\n  hint: run `go test ./formatter/... -run TestGolden -update` to generate", err)
			}
			if got != string(want) {
				t.Errorf("output mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, want, got)
			}
		})
	}
}
