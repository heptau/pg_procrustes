package formatter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
	"gopkg.in/yaml.v3"
)

// styleYAML is the opinionated style most tests were written against. An empty
// config is a no-op (every rule "preserve"), so tests that exercise formatting
// without naming each option layer their own settings on top of this.
const styleYAML = `
reserved_keywords: {case: upper}
keywords: {case: upper}
data_types: {case: lower, form: long}
literals: {case: upper}
operators: {case: upper}
schemas: {case: lower}
tables: {case: lower}
functions: {case: lower}
conditional_functions: {case: upper}
system_functions: {case: upper}
aliases: {case: lower, as: add}
columns: {case: lower}
trailing_whitespace: strip
inequality_op: c
operator_spacing: normalize
paren_spacing: remove
quoted_identifiers: remove_safe
comma_spacing: normalize
layout: {indent: {size: 3}}
`

func mergeMaps(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				mergeMaps(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}

// loadStyled loads path (may be "" or missing) on top of styleYAML.
func loadStyled(path string) (*config.Config, error) {
	base := map[string]any{}
	if err := yaml.Unmarshal([]byte(styleYAML), &base); err != nil {
		return nil, err
	}
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			over := map[string]any{}
			if err := yaml.Unmarshal(data, &over); err != nil {
				return nil, err
			}
			mergeMaps(base, over)
		}
	}
	out, err := yaml.Marshal(base)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "pgp_style_*.yaml")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(out); err != nil {
		return nil, err
	}
	_ = f.Close()
	return config.Load(f.Name())
}

// An empty config must not change valid SQL at all.
func TestEmptyConfigIsNoop(t *testing.T) {
	cfg, err := config.Load("/nonexistent/.pg_procrustes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob("testdata/*/input.sql")
	if len(files) == 0 {
		t.Fatal("no golden inputs found")
	}
	for _, f := range files {
		src, _ := os.ReadFile(f)
		got, err := formatter.Format(string(src), cfg)
		if err != nil {
			continue // inputs that do not parse are covered by the syntax_errors tests
		}
		if got != string(src) {
			t.Errorf("%s: empty config changed the input\n--- got ---\n%s", f, got)
		}
	}
}
