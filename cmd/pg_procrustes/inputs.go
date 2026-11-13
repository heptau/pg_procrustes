package main

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// input is one file to format. rel is where its output goes below --out-dir: the file's
// name, or its path below the directory (or the fixed part of the pattern) it was found in.
type input struct {
	path string
	rel  string
}

// expandInputs turns the command-line arguments into files:
//   - a directory is searched recursively for *.sql files;
//   - a pattern with ** matches any number of directories (migrations/**/*.sql);
//   - other patterns are ordinary globs (a match that is a directory is searched like one);
//   - anything else — a file, or a name that matches nothing — is kept as it is, so the
//     open error surfaces naturally.
//
// Hidden directories (.git, …) and node_modules are not entered. Files named on the command
// line are formatted whatever their extension. The result has no duplicates.
func expandInputs(args []string) ([]input, error) {
	var out []input
	seen := map[string]bool{}
	add := func(p, rel string) {
		key := filepath.Clean(p)
		if !seen[key] {
			seen[key] = true
			out = append(out, input{path: p, rel: rel})
		}
	}
	addDir := func(dir string) error {
		files, err := sqlFilesBelow(dir)
		if err != nil {
			return err
		}
		for _, f := range files {
			rel, _ := filepath.Rel(dir, f)
			add(f, rel)
		}
		return nil
	}
	for _, arg := range args {
		if info, err := os.Stat(arg); err == nil && info.IsDir() {
			if err := addDir(arg); err != nil {
				return nil, err
			}
			continue
		}
		if info, err := os.Stat(arg); err == nil && !info.IsDir() {
			// An existing file is that file, even if its name looks like a pattern
			// (v[1].sql would otherwise match v1.sql instead).
			add(arg, filepath.Base(arg))
			continue
		}
		if strings.Contains(filepath.ToSlash(arg), "**") {
			base, files, err := globStar(arg)
			if err != nil {
				return nil, err
			}
			for _, f := range files {
				rel, _ := filepath.Rel(base, f)
				add(f, rel)
			}
			if len(files) == 0 {
				add(arg, filepath.Base(arg))
			}
			continue
		}
		matches, err := filepath.Glob(arg)
		if err != nil {
			return nil, &globError{arg, err}
		}
		if len(matches) == 0 {
			add(arg, filepath.Base(arg))
			continue
		}
		for _, m := range matches {
			if info, err := os.Stat(m); err == nil && info.IsDir() {
				if err := addDir(m); err != nil {
					return nil, err
				}
				continue
			}
			add(m, filepath.Base(m))
		}
	}
	return out, nil
}

type globError struct {
	pattern string
	err     error
}

func (e *globError) Error() string { return "invalid glob " + `"` + e.pattern + `": ` + e.err.Error() }

// skipDir reports whether the recursive search leaves the directory name alone.
func skipDir(name string) bool {
	return name == "node_modules" || (strings.HasPrefix(name, ".") && name != "." && name != "..")
}

// sqlFilesBelow returns the *.sql files below dir (any depth), sorted by path.
func sqlFilesBelow(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ".sql") && d.Type().IsRegular() {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func hasGlobMeta(s string) bool { return strings.ContainsAny(s, "*?[\\") }

// globStar matches pattern, in which a path segment ** stands for any number of directories
// (also none), against the files below the fixed leading part of the pattern. It returns
// that directory and the sorted matches.
func globStar(pattern string) (string, []string, error) {
	segs := strings.Split(filepath.ToSlash(pattern), "/")
	fixed := 0
	for fixed < len(segs)-1 && !hasGlobMeta(segs[fixed]) {
		fixed++
	}
	base := path.Join(segs[:fixed]...)
	if base == "" {
		base = "."
		if strings.HasPrefix(pattern, "/") {
			base = "/"
		}
	} else if strings.HasPrefix(pattern, "/") && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	rest := segs[fixed:]
	if _, err := path.Match(strings.Join(rest, "/"), ""); err != nil {
		return "", nil, &globError{pattern, err}
	}
	var files []string
	err := filepath.WalkDir(filepath.FromSlash(base), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() {
			if p != filepath.FromSlash(base) && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(filepath.FromSlash(base), p)
		if d.Type().IsRegular() && matchSegments(rest, strings.Split(filepath.ToSlash(rel), "/")) {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	return filepath.FromSlash(base), files, err
}

// matchSegments matches the path segments name against the pattern segments pat, where **
// matches zero or more segments.
func matchSegments(pat, name []string) bool {
	if len(pat) == 0 {
		return len(name) == 0
	}
	if pat[0] == "**" {
		for k := 0; k <= len(name); k++ {
			if matchSegments(pat[1:], name[k:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	if ok, _ := path.Match(pat[0], name[0]); !ok {
		return false
	}
	return matchSegments(pat[1:], name[1:])
}
