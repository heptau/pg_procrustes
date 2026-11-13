package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRejectsBadPerClauseRules(t *testing.T) {
	cfg := defaultConfig()
	cfg.Layout.Clauses.When.Break = "sometimes"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected an error for layout.clauses.when.break")
	}
	cfg = defaultConfig()
	cfg.Layout.Clauses.Where.Align = "sideways"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected an error for layout.clauses.where.align")
	}
	cfg = defaultConfig()
	cfg.Layout.Content.WhenActions.Break = "bogus"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected an error for layout.content.when_actions.break")
	}
	if err := Validate(defaultConfig()); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
}

func TestValidateRejectsBadPositions(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"comma_position":            func(c *Config) { c.Layout.Content.CommaPosition = "middle" },
		"logical_operator_position": func(c *Config) { c.Layout.Content.LogicalOperatorPosition = "middle" },
		"join_on_position":          func(c *Config) { c.Layout.Content.JoinOnPosition = "middle" },
	} {
		cfg := defaultConfig()
		mutate(cfg)
		if err := Validate(cfg); err == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

func TestValidateRejectsBadDDLSettings(t *testing.T) {
	cfg := defaultConfig()
	cfg.Layout.DDL.CreateTable.Break = "sometimes"
	if Validate(cfg) == nil {
		t.Error("expected an error for layout.ddl.create_table.break")
	}
	cfg = defaultConfig()
	cfg.Layout.DDL.CreateTable.Align = "columns"
	if Validate(cfg) == nil {
		t.Error("expected an error for layout.ddl.create_table.align")
	}
	cfg = defaultConfig()
	cfg.Layout.DDL.AlterTable.Break = "x"
	if Validate(cfg) == nil {
		t.Error("expected an error for layout.ddl.alter_table.break")
	}
}

// A negative indent size made the layout panic (strings.Repeat); numbers are validated.
func TestValidateRejectsBadNumbers(t *testing.T) {
	cfg := defaultConfig()
	cfg.Layout.Indent.Size = -3
	if err := Validate(cfg); err == nil {
		t.Fatal("expected an error for a negative layout.indent.size")
	}
	cfg = defaultConfig()
	cfg.Layout.Indent.Size = 1000000
	if err := Validate(cfg); err == nil {
		t.Fatal("expected an error for a huge layout.indent.size")
	}
	cfg = defaultConfig()
	cfg.Layout.LineLength = -1
	if err := Validate(cfg); err == nil {
		t.Fatal("expected an error for a negative layout.line_length")
	}
}

// An option that does not exist (a typo) is an error that names it, with and without a
// template; an empty file and the `template` key are fine.
func TestLoadRejectsUnknownOptions(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	base := write("base.yaml", "keywords:\n  case: upper\n")
	for name, tc := range map[string]struct{ text, want string }{
		"top.yaml":    {"keywordz:\n  case: upper\n", `line 1: unknown option "keywordz"`},
		"nested.yaml": {"keywords:\n  casee: upper\n", `line 2: unknown option "casee"`},
		"layout.yaml": {"layout:\n  indent:\n    sizes: 2\n", `line 3: unknown option "sizes"`},
		"tmpl.yaml":   {"template: base.yaml\nlayout:\n  line_limit: 80\n", `tmpl.yaml: line 3: unknown option "line_limit"`},
	} {
		_, err := Load(write(name, tc.text))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error with %q", name, err, tc.want)
		}
	}
	// The file that has the typo is named, not the one that uses it as a template.
	bad := write("bad.yaml", "keywordz:\n  case: upper\n")
	_, err := Load(write("uses_bad.yaml", "template: bad.yaml\n"))
	if err == nil || !strings.Contains(err.Error(), bad) {
		t.Errorf("want an error naming %s, got %v", bad, err)
	}
	for name, text := range map[string]string{
		"empty.yaml":   "",
		"comment.yaml": "# nothing\n",
		"ok.yaml":      "template: base.yaml\nlayout:\n  line_length: 80\n",
	} {
		if _, err := Load(write(name, text)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	c, err := Load(base)
	if err != nil || c.Keywords.Case != CaseUpper {
		t.Errorf("base.yaml: %v, %v", c, err)
	}
}
