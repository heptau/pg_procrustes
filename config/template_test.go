package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// templatesDir makes an installed-templates directory known through the environment.
func templatesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PG_PROCRUSTES_TEMPLATES", dir)
	return dir
}

func TestTemplateByNameThenOwnSettingsWin(t *testing.T) {
	tpl := templatesDir(t)
	write(t, tpl, "base.yaml", "keywords:\n  case: lower\nlayout:\n  indent:\n    size: 4\n    type: tab\n")
	cfgPath := write(t, t.TempDir(), ".pg_procrustes.yaml", "template: base\nlayout:\n  indent:\n    size: 6\n")
	c, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if c.Keywords.Case != CaseLower || c.Layout.Indent.Type != IndentTypeTab {
		t.Errorf("template values missing: %+v", c.Layout.Indent)
	}
	if c.Layout.Indent.Size != 6 {
		t.Errorf("own setting did not win: size %d", c.Layout.Indent.Size)
	}
	if len(c.Sources) != 2 || !strings.HasSuffix(c.Sources[0], "base.yaml") || c.Sources[1] != cfgPath {
		t.Errorf("sources: %v", c.Sources)
	}
}

func TestTemplateListAppliedInOrder(t *testing.T) {
	tpl := templatesDir(t)
	write(t, tpl, "a.yaml", "keywords:\n  case: lower\nliterals:\n  case: lower\n")
	write(t, tpl, "b.yml", "keywords:\n  case: upper\n")
	cfgPath := write(t, t.TempDir(), "c.yaml", "template: [a, b]\n")
	c, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if c.Keywords.Case != CaseUpper || c.Literals.Case != CaseLower {
		t.Errorf("later template must win, earlier ones stay: keywords=%s literals=%s", c.Keywords.Case, c.Literals.Case)
	}
}

func TestTemplateFileRelativeToTheFileThatNamesIt(t *testing.T) {
	root := t.TempDir()
	write(t, root, "shared/team.yaml", "template: ./inner.yaml\nkeywords:\n  case: lower\n")
	write(t, root, "shared/inner.yaml", "literals:\n  case: lower\nkeywords:\n  case: upper\n")
	cfgPath := write(t, root, "project/.pg_procrustes.yaml", "template: ../shared/team.yaml\n")
	c, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if c.Keywords.Case != CaseLower || c.Literals.Case != CaseLower {
		t.Errorf("chain not applied: %s %s", c.Keywords.Case, c.Literals.Case)
	}
	if len(c.Sources) != 3 {
		t.Errorf("sources: %v", c.Sources)
	}
}

func TestTemplateListsAreReplacedNotMerged(t *testing.T) {
	tpl := templatesDir(t)
	write(t, tpl, "base.yaml", "keywords:\n  exceptions: [a, b]\n")
	cfgPath := write(t, t.TempDir(), "c.yaml", "template: base\nkeywords:\n  exceptions: [c]\n")
	c, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Keywords.Exceptions, []string{"c"}) {
		t.Errorf("exceptions: %v", c.Keywords.Exceptions)
	}
}

func TestTemplateErrors(t *testing.T) {
	templatesDir(t)
	dir := t.TempDir()
	if _, err := Load(write(t, dir, "missing.yaml", "template: nope\n")); err == nil || !strings.Contains(err.Error(), `template "nope" not found`) {
		t.Errorf("missing template: %v", err)
	}
	if _, err := Load(write(t, dir, "file.yaml", "template: ./nofile.yaml\n")); err == nil {
		t.Errorf("missing template file must fail")
	}
	write(t, dir, "a.yaml", "template: ./b.yaml\n")
	write(t, dir, "b.yaml", "template: ./a.yaml\n")
	if _, err := Load(filepath.Join(dir, "a.yaml")); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("cycle: %v", err)
	}
	if _, err := Load(write(t, dir, "bad.yaml", "template: {x: 1}\n")); err == nil {
		t.Errorf("a template that is not a name or list must fail")
	}
	// the merged result is validated like any config
	tpl := os.Getenv("PG_PROCRUSTES_TEMPLATES")
	write(t, tpl, "broken.yaml", "keywords:\n  case: shouting\n")
	if _, err := Load(write(t, dir, "usebroken.yaml", "template: broken\n")); err == nil {
		t.Errorf("invalid value in a template must fail validation")
	}
}

func TestListTemplatesEarlierDirectoryWins(t *testing.T) {
	d1, d2 := t.TempDir(), t.TempDir()
	t.Setenv("PG_PROCRUSTES_TEMPLATES", d1+string(os.PathListSeparator)+d2)
	write(t, d1, "x.yaml", "")
	write(t, d2, "x.yaml", "")
	write(t, d2, "y.yml", "")
	write(t, d2, "not a name.yaml", "")
	got := ListTemplates()
	if !strings.HasPrefix(got["x"], d1) || got["y"] == "" || len(got) != 2 {
		t.Errorf("templates: %v", got)
	}
}

func TestEffectiveConfigRoundTrips(t *testing.T) {
	tpl := templatesDir(t)
	write(t, tpl, "base.yaml", "keywords:\n  case: lower\nlayout:\n  content:\n    break: always\n    comma_position: start\n")
	c, err := Load(write(t, t.TempDir(), "c.yaml", "template: base\nstatement_break: always\n"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.YAML()
	if err != nil {
		t.Fatal(err)
	}
	again, err := Load(write(t, t.TempDir(), "again.yaml", string(out)))
	if err != nil {
		t.Fatalf("printed config does not load: %v\n%s", err, out)
	}
	// compare the rendered form: nil and empty exception lists are the same thing
	out2, err := again.YAML()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(out2) {
		t.Errorf("print-config output does not reproduce the config\n--- first ---\n%s\n--- second ---\n%s", out, out2)
	}
}

// Every template shipped in templates/ must load and validate.
func TestShippedTemplatesLoad(t *testing.T) {
	files, _ := filepath.Glob("../templates/*.yaml")
	if len(files) == 0 {
		t.Fatal("no templates found in templates/")
	}
	for _, f := range files {
		c, err := Load(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if err := Validate(c); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

// The reference config in the repository root documents every option: it must load, validate
// and name no key the program does not know (a renamed or removed option would otherwise
// linger in it unnoticed).
func TestReferenceConfigIsCurrent(t *testing.T) {
	const path = "../.pg_procrustes.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(defaultConfig()); err != nil {
		t.Errorf("%s names an option the program does not have: %v", path, err)
	}
}
