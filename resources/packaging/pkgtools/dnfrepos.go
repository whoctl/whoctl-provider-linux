package pkgtools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dnfRepos manages the .repo files under /etc/yum.repos.d.
//
// These are INI files: a `[id]` section header followed by key=value lines, and
// one file may hold several sections. The id is the repository's real identity —
// it is what `dnf --enablerepo` takes — so it is the object name, and the file
// is just where it happens to live.
//
// Keys whoctl does not model are preserved exactly where they were: a real
// .repo file carries `skip_if_unavailable`, `countme`, module hints and
// whatever else the vendor put there, and rewriting the file from the model
// alone would silently drop all of it.
type dnfRepos struct{ opts Options }

func (b *dnfRepos) Name() string { return "dnf" }

func (b *dnfRepos) ConfigPath() string {
	return filepath.Join(rootOr(b.opts.Root), "etc/yum.repos.d")
}

func (b *dnfRepos) List(ctx context.Context) ([]Repository, error) {
	files, err := b.files()
	if err != nil {
		return nil, err
	}
	var out []Repository
	for _, path := range files {
		lines, _, err := readLines(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		for _, section := range parseINI(lines) {
			out = append(out, iniToRepository(section, path))
		}
	}
	sortRepos(out)
	return out, nil
}

func (b *dnfRepos) files() ([]string, error) {
	entries, err := os.ReadDir(b.ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", b.ConfigPath(), err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".repo") {
			out = append(out, filepath.Join(b.ConfigPath(), e.Name()))
		}
	}
	return out, nil
}

// iniSection is one [id] block: its key order is kept so a rewrite does not
// reshuffle a file the vendor wrote.
type iniSection struct {
	name  string
	keys  []string
	value map[string]string
	// leading holds the comment and blank lines that came before the header.
	leading []string
}

func parseINI(lines []string) []iniSection {
	var (
		sections []iniSection
		current  *iniSection
		pending  []string
	)
	for _, line := range lines {
		text := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]"):
			if current != nil {
				sections = append(sections, *current)
			}
			current = &iniSection{
				name:    strings.Trim(text, "[]"),
				value:   map[string]string{},
				leading: pending,
			}
			pending = nil
		case current == nil, text == "", strings.HasPrefix(text, "#"), strings.HasPrefix(text, ";"):
			pending = append(pending, line)
		default:
			key, value, ok := strings.Cut(text, "=")
			if !ok {
				pending = append(pending, line)
				continue
			}
			key = strings.TrimSpace(key)
			if _, seen := current.value[key]; !seen {
				current.keys = append(current.keys, key)
			}
			current.value[key] = strings.TrimSpace(value)
		}
	}
	if current != nil {
		sections = append(sections, *current)
	}
	return sections
}

func iniToRepository(s iniSection, path string) Repository {
	repo := Repository{
		Name:        s.name,
		File:        path,
		DisplayName: s.value["name"],
		Metalink:    s.value["metalink"],
		Mirrorlist:  s.value["mirrorlist"],
		GPGKey:      s.value["gpgkey"],
		// dnf treats a repository with no enabled key as enabled.
		Enabled: iniBool(s.value, "enabled", true),
	}
	if baseurl := s.value["baseurl"]; baseurl != "" {
		repo.URLs = strings.Fields(baseurl)
	}
	if raw, ok := s.value["gpgcheck"]; ok {
		check := iniTruthy(raw)
		repo.GPGCheck = &check
	}
	return repo
}

func iniBool(values map[string]string, key string, fallback bool) bool {
	raw, ok := values[key]
	if !ok {
		return fallback
	}
	return iniTruthy(raw)
}

// iniTruthy accepts everything the Red Hat tooling writes for a boolean.
func iniTruthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (b *dnfRepos) Apply(ctx context.Context, repo Repository) (bool, error) {
	path, err := b.fileFor(repo)
	if err != nil {
		return false, err
	}
	lines, _, err := readLines(path)
	if err != nil {
		return false, err
	}
	sections := parseINI(lines)

	for i, s := range sections {
		if s.name != repo.Name {
			continue
		}
		updated := applyToSection(s, repo)
		if sameSection(s, updated) {
			return false, nil
		}
		sections[i] = updated
		return true, writeFile(path, renderINI(sections), 0o644)
	}

	created := applyToSection(iniSection{name: repo.Name, value: map[string]string{}}, repo)
	return true, writeFile(path, renderINI(append(sections, created)), 0o644)
}

// fileFor decides which .repo file an entry belongs to: the one it already
// lives in, the one the manifest asked for, or <id>.repo.
func (b *dnfRepos) fileFor(repo Repository) (string, error) {
	if repo.File != "" {
		if filepath.IsAbs(repo.File) {
			return repo.File, nil
		}
		return filepath.Join(b.ConfigPath(), repo.File), nil
	}
	existing, found, err := GetRepo(context.Background(), b, repo.Name)
	if err != nil {
		return "", err
	}
	if found {
		return existing.File, nil
	}
	return filepath.Join(b.ConfigPath(), repo.Name+".repo"), nil
}

// applyToSection writes the modelled keys into a section, leaving every other
// key exactly as it was.
func applyToSection(s iniSection, repo Repository) iniSection {
	out := iniSection{name: s.name, keys: append([]string(nil), s.keys...), value: map[string]string{}, leading: s.leading}
	for k, v := range s.value {
		out.value[k] = v
	}

	set := func(key, value string) {
		if value == "" {
			return
		}
		if _, seen := out.value[key]; !seen {
			out.keys = append(out.keys, key)
		}
		out.value[key] = value
	}
	set("name", repo.DisplayName)
	set("baseurl", strings.Join(repo.URLs, " "))
	set("metalink", repo.Metalink)
	set("mirrorlist", repo.Mirrorlist)
	set("gpgkey", repo.GPGKey)
	set("enabled", boolToINI(repo.Enabled))
	if repo.GPGCheck != nil {
		set("gpgcheck", boolToINI(*repo.GPGCheck))
	}
	return out
}

func boolToINI(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func sameSection(a, b iniSection) bool {
	if len(a.value) != len(b.value) {
		return false
	}
	for k, v := range a.value {
		if b.value[k] != v {
			return false
		}
	}
	return true
}

func renderINI(sections []iniSection) string {
	var out []string
	for i, s := range sections {
		out = append(out, s.leading...)
		if i > 0 && len(s.leading) == 0 {
			out = append(out, "")
		}
		out = append(out, "["+s.name+"]")
		for _, k := range s.keys {
			out = append(out, k+"="+s.value[k])
		}
	}
	return joinLines(out)
}

func (b *dnfRepos) Delete(ctx context.Context, name string) error {
	files, err := b.files()
	if err != nil {
		return err
	}
	for _, path := range files {
		lines, _, err := readLines(path)
		if err != nil {
			return err
		}
		sections := parseINI(lines)
		kept := make([]iniSection, 0, len(sections))
		for _, s := range sections {
			if s.name != name {
				kept = append(kept, s)
			}
		}
		if len(kept) == len(sections) {
			continue
		}
		// A file that held nothing but this repository is removed outright,
		// rather than left behind as an empty stub dnf would still read.
		if len(kept) == 0 {
			return os.Remove(path)
		}
		return writeFile(path, renderINI(kept), 0o644)
	}
	return nil
}
