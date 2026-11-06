package config

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ReleaseRepo is the GitHub repository whose releases carry templates.zip.
const ReleaseRepo = "heptau/pg_procrustes"

const (
	maxTemplatesZip  = 4 << 20   // download / archive size limit
	maxTemplateFile  = 256 << 10 // size limit of one template
	maxTemplateFiles = 200
)

var templateFileRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*\.ya?ml$`)

// UserTemplateDir is where InstallTemplates puts templates: the first entry of
// $PG_PROCRUSTES_TEMPLATES when set, else <user config dir>/pg_procrustes/templates.
func UserTemplateDir() (string, error) {
	if env := os.Getenv("PG_PROCRUSTES_TEMPLATES"); env != "" {
		if list := filepath.SplitList(env); len(list) > 0 && list[0] != "" {
			return list[0], nil
		}
	}
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("no user config directory: %w", err)
	}
	return filepath.Join(cfgDir, "pg_procrustes", "templates"), nil
}

// TemplatesURL is the templates.zip of the release matching version ("v0.3.0" or "0.3.0");
// a version that is not a release (dev builds) gets the latest release.
func TemplatesURL(version string) string {
	v := strings.TrimPrefix(version, "v")
	if regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(v) {
		return "https://github.com/" + ReleaseRepo + "/releases/download/v" + v + "/templates.zip"
	}
	return "https://github.com/" + ReleaseRepo + "/releases/latest/download/templates.zip"
}

// InstalledTemplate is one file written by InstallTemplates.
type InstalledTemplate struct {
	Name     string // template name (file name without extension)
	Path     string
	Replaced bool // an older file of that name was overwritten
}

// InstallTemplates puts the templates found in src into dest. src is an http(s) URL of a
// zip, a local .zip file or a directory; templates are the *.yaml files at the top of the
// zip / directory (or below a single templates/ folder). Only plain file names that are valid
// template names are accepted, each file must be a YAML mapping, and size limits apply.
func InstallTemplates(src, dest string) ([]InstalledTemplate, error) {
	files, err := readTemplateSource(src)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no templates (*.yaml) found in %s", src)
	}
	for name, data := range files {
		var probe map[string]any
		if err := yaml.Unmarshal(data, &probe); err != nil || probe == nil {
			return nil, fmt.Errorf("%s is not a YAML mapping", name)
		}
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []InstalledTemplate
	for _, n := range names {
		target := filepath.Join(dest, n)
		_, statErr := os.Stat(target)
		tmp, err := os.CreateTemp(dest, ".tmpl-*")
		if err != nil {
			return out, err
		}
		_, werr := tmp.Write(files[n])
		cerr := tmp.Close()
		if werr != nil || cerr != nil {
			_ = os.Remove(tmp.Name())
			return out, fmt.Errorf("writing %s: %v %v", n, werr, cerr)
		}
		if err := os.Chmod(tmp.Name(), 0o644); err != nil {
			_ = os.Remove(tmp.Name())
			return out, err
		}
		if err := os.Rename(tmp.Name(), target); err != nil {
			_ = os.Remove(tmp.Name())
			return out, err
		}
		out = append(out, InstalledTemplate{
			Name:     strings.TrimSuffix(strings.TrimSuffix(n, ".yaml"), ".yml"),
			Path:     target,
			Replaced: statErr == nil,
		})
	}
	return out, nil
}

func readTemplateSource(src string) (map[string][]byte, error) {
	switch {
	case strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "http://"):
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(src)
		if err != nil {
			return nil, fmt.Errorf("download %s: %w", src, err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			hint := ""
			if resp.StatusCode == http.StatusNotFound {
				hint = " (that release has no templates.zip; use --templates-from with a zip or a templates/ folder)"
			}
			return nil, fmt.Errorf("download %s: %s%s", src, resp.Status, hint)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxTemplatesZip+1))
		if err != nil {
			return nil, fmt.Errorf("download %s: %w", src, err)
		}
		if len(data) > maxTemplatesZip {
			return nil, fmt.Errorf("download %s: larger than %d bytes", src, maxTemplatesZip)
		}
		return readTemplateZip(data)
	}
	info, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(src)
		if err != nil {
			return nil, err
		}
		files := map[string][]byte{}
		for _, e := range entries {
			if e.IsDir() || !templateFileRE.MatchString(e.Name()) {
				continue
			}
			data, err := readLimited(filepath.Join(src, e.Name()))
			if err != nil {
				return nil, err
			}
			files[e.Name()] = data
			if len(files) > maxTemplateFiles {
				return nil, fmt.Errorf("more than %d templates in %s", maxTemplateFiles, src)
			}
		}
		return files, nil
	}
	if info.Size() > maxTemplatesZip {
		return nil, fmt.Errorf("%s is larger than %d bytes", src, maxTemplatesZip)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	return readTemplateZip(data)
}

func readLimited(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxTemplateFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxTemplateFile {
		return nil, fmt.Errorf("%s is larger than %d bytes", p, maxTemplateFile)
	}
	return data, nil
}

// readTemplateZip returns the template files of a zip: *.yaml at its top level or directly
// below one templates/ folder. Everything else (other folders, odd names, ..) is ignored.
func readTemplateZip(data []byte) (map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a zip archive: %w", err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.ReplaceAll(f.Name, "\\", "/")
		dir, base := path.Split(name)
		if dir != "" && dir != "templates/" {
			continue
		}
		if !templateFileRE.MatchString(base) {
			continue
		}
		if f.UncompressedSize64 > maxTemplateFile {
			return nil, fmt.Errorf("%s is larger than %d bytes", f.Name, maxTemplateFile)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxTemplateFile+1))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if len(b) > maxTemplateFile {
			return nil, fmt.Errorf("%s is larger than %d bytes", f.Name, maxTemplateFile)
		}
		files[base] = b
		if len(files) > maxTemplateFiles {
			return nil, fmt.Errorf("more than %d templates in the archive", maxTemplateFiles)
		}
	}
	return files, nil
}
