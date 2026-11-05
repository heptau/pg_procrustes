package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// A config file can start from other config files:
//
//	template: ai                          # an installed template, by name
//	template: ../shared/team.yaml         # a file, relative to the file that names it
//	template: [ai, ../shared/team.yaml]   # several, applied in this order (later ones win)
//
// The settings of the file itself are applied last, so they always win. Maps are merged
// key by key, any other value (including lists such as `exceptions`) is replaced.
// Templates are ordinary config files and may name templates themselves.

// templateNameRE is what an installed template is called: no path separators, no extension.
var templateNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// TemplateDirs lists where installed templates are looked up, first match wins:
// $PG_PROCRUSTES_TEMPLATES, the user's config directory (…/pg_procrustes/templates),
// then the directories shipped with the binary (templates/ next to it and
// ../share/pg_procrustes/templates).
func TemplateDirs() []string {
	var dirs []string
	if env := os.Getenv("PG_PROCRUSTES_TEMPLATES"); env != "" {
		dirs = append(dirs, filepath.SplitList(env)...)
	}
	if cfgDir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(cfgDir, "pg_procrustes", "templates"))
	}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		dir := filepath.Dir(exe)
		dirs = append(dirs,
			filepath.Join(dir, "templates"),
			filepath.Join(dir, "..", "share", "pg_procrustes", "templates"))
	}
	return dirs
}

// ListTemplates returns the installed template names with the file each one resolves to.
func ListTemplates() map[string]string {
	found := map[string]string{}
	for _, dir := range TemplateDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".yaml"), ".yml")
			if e.IsDir() || name == e.Name() || !templateNameRE.MatchString(name) {
				continue
			}
			if _, ok := found[name]; !ok { // earlier directories win
				found[name] = filepath.Join(dir, e.Name())
			}
		}
	}
	return found
}

// resolveTemplate turns a `template:` entry into a file path.
func resolveTemplate(ref, baseDir string) (string, error) {
	if templateNameRE.MatchString(ref) {
		dirs := TemplateDirs()
		for _, dir := range dirs {
			for _, ext := range []string{".yaml", ".yml"} {
				p := filepath.Join(dir, ref+ext)
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					return p, nil
				}
			}
		}
		return "", fmt.Errorf("template %q not found (looked in: %s; set PG_PROCRUSTES_TEMPLATES to add a directory)", ref, strings.Join(dirs, ", "))
	}
	p := ref
	if !filepath.IsAbs(p) {
		p = filepath.Join(baseDir, p)
	}
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("template file %q: %w", ref, err)
	}
	return p, nil
}

// templateRefs reads the value of the `template` key: a string or a list of strings.
func templateRefs(v any) ([]string, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		return []string{t}, nil
	case []any:
		refs := make([]string, 0, len(t))
		for _, x := range t {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("template must be a name, a path or a list of them")
			}
			refs = append(refs, s)
		}
		return refs, nil
	}
	return nil, fmt.Errorf("template must be a name, a path or a list of them")
}

// mergeMaps copies src onto dst: maps are merged recursively, everything else replaces.
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

// loadMerged reads path (data is its content), applies the templates it names and then its
// own settings. files collects every file read, in the order they are applied.
func loadMerged(path string, data []byte, stack []string, files *[]string) (map[string]any, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	for _, s := range stack {
		if s == abs {
			return nil, fmt.Errorf("templates form a cycle: %s", strings.Join(append(stack, abs), " -> "))
		}
	}
	stack = append(stack, abs)

	var own map[string]any
	if err := yaml.Unmarshal(data, &own); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if own == nil {
		own = map[string]any{}
	}
	refs, err := templateRefs(own["template"])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	delete(own, "template")

	merged := map[string]any{}
	for _, ref := range refs {
		tp, err := resolveTemplate(ref, filepath.Dir(abs))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		tdata, err := os.ReadFile(tp)
		if err != nil {
			return nil, err
		}
		tm, err := loadMerged(tp, tdata, stack, files)
		if err != nil {
			return nil, err
		}
		mergeMaps(merged, tm)
	}
	mergeMaps(merged, own)
	*files = append(*files, abs)
	return merged, nil
}

// hasTemplate reports whether the YAML document has a top-level `template` key.
func hasTemplate(data []byte) bool {
	var probe map[string]any
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return false
	}
	_, ok := probe["template"]
	return ok
}

// YAML renders the effective configuration with every option (what --print-config shows).
func (c *Config) YAML() ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sortedKeys is a small helper for deterministic output.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SortedTemplateNames returns the installed template names in alphabetical order.
func SortedTemplateNames(t map[string]string) []string { return sortedKeys(t) }

// MergedSettings reads a config file and applies the templates it names, returning the
// merged generic document (without the `template` key) and the files read, in order.
// It is what the config builder is generated from.
func MergedSettings(path string) (map[string]any, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var files []string
	m, err := loadMerged(path, data, nil, &files)
	return m, files, err
}
