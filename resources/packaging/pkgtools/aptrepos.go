package pkgtools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// aptRepos manages the files under /etc/apt/sources.list.d, plus the classic
// /etc/apt/sources.list.
//
// The object name is the file's base name without its extension, because that
// is the identity apt users already work with: you add a repository by dropping
// `docker.list` in the directory and remove it by deleting the file. A source
// line has no id of its own to use instead.
//
// Two formats are in the wild. The one-line format — `deb [opts] uri suite
// components` — is what whoctl writes. The deb822 format that Debian 12 and
// later ship, one `.sources` file of RFC822 stanzas, is read so those
// repositories are visible, but not rewritten: a stanza can hold several URIs
// and suites at once, which the single-entry model here cannot represent
// without quietly dropping some of it.
type aptRepos struct{ opts Options }

func (b *aptRepos) Name() string { return "apt" }

func (b *aptRepos) ConfigPath() string {
	return filepath.Join(rootOr(b.opts.Root), "etc/apt/sources.list.d")
}

func (b *aptRepos) mainList() string {
	return filepath.Join(rootOr(b.opts.Root), "etc/apt/sources.list")
}

func (b *aptRepos) List(ctx context.Context) ([]Repository, error) {
	var out []Repository
	for _, path := range b.files() {
		repo, ok, err := b.read(path)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, repo)
		}
	}
	sortRepos(out)
	return out, nil
}

func (b *aptRepos) files() []string {
	var paths []string
	if _, err := os.Stat(b.mainList()); err == nil {
		paths = append(paths, b.mainList())
	}
	entries, err := os.ReadDir(b.ConfigPath())
	if err != nil {
		return paths
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(name, ".list") || strings.HasSuffix(name, ".sources") {
			paths = append(paths, filepath.Join(b.ConfigPath(), name))
		}
	}
	return paths
}

// read turns one file into a repository. A file with no active source line at
// all is skipped: apt's directory routinely holds disabled leftovers and
// `.list` files that are nothing but comments.
func (b *aptRepos) read(path string) (Repository, bool, error) {
	lines, exists, err := readLines(path)
	if err != nil {
		return Repository{}, false, fmt.Errorf("reading %s: %w", path, err)
	}
	if !exists {
		return Repository{}, false, nil
	}

	repo := Repository{Name: aptRepoName(path), File: path}
	if strings.HasSuffix(path, ".sources") {
		return parseDeb822(lines, repo)
	}

	found := false
	for _, line := range lines {
		entry, enabled, ok := parseAptLine(line)
		if !ok {
			continue
		}
		// The first source line in the file defines the object; a later one
		// only marks the file as holding more than whoctl models.
		if found {
			repo.Options["extraEntries"] = "true"
			break
		}
		entry.Name, entry.File = repo.Name, repo.File
		entry.Enabled = enabled
		repo = entry
		found = true
	}
	return repo, found, nil
}

// aptRepoName strips the directory and the extension: sources.list.d/docker.list
// becomes "docker", and /etc/apt/sources.list becomes "sources".
func aptRepoName(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".list")
	base = strings.TrimSuffix(base, ".sources")
	return base
}

// parseAptLine reads one `deb [options] uri suite [components...]` line.
func parseAptLine(line string) (Repository, bool, bool) {
	text := strings.TrimSpace(line)
	enabled := true
	if strings.HasPrefix(text, "#") {
		enabled = false
		text = strings.TrimSpace(strings.TrimLeft(text, "#"))
	}
	if text == "" {
		return Repository{}, false, false
	}

	fields := strings.Fields(text)
	if len(fields) < 3 || (fields[0] != "deb" && fields[0] != "deb-src") {
		return Repository{}, false, false
	}

	repo := Repository{Types: []string{fields[0]}, Options: map[string]string{}}
	fields = fields[1:]

	// Options are bracketed and space-separated: [arch=amd64 signed-by=/k.gpg].
	if strings.HasPrefix(fields[0], "[") {
		var opts []string
		for len(fields) > 0 {
			part := fields[0]
			fields = fields[1:]
			opts = append(opts, strings.Trim(part, "[]"))
			if strings.HasSuffix(part, "]") {
				break
			}
		}
		for _, opt := range opts {
			if k, v, ok := strings.Cut(opt, "="); ok {
				repo.Options[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	if len(fields) < 2 {
		return Repository{}, false, false
	}
	repo.URLs = []string{fields[0]}
	repo.Suite = fields[1]
	repo.Components = fields[2:]
	return repo, enabled, true
}

// parseDeb822 reads the modern stanza format well enough to report what is
// there. Only the first stanza is described, and the entry is marked read-only.
func parseDeb822(lines []string, repo Repository) (Repository, bool, error) {
	repo.Options = map[string]string{"format": "deb822"}
	repo.Enabled = true
	for _, line := range lines {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(key) {
		case "types":
			repo.Types = strings.Fields(value)
		case "uris":
			repo.URLs = strings.Fields(value)
		case "suites":
			repo.Suite = value
		case "components":
			repo.Components = strings.Fields(value)
		case "signed-by":
			repo.Options["signed-by"] = value
		case "enabled":
			repo.Enabled = !strings.EqualFold(value, "no")
		}
	}
	return repo, len(repo.URLs) > 0, nil
}

func (b *aptRepos) Apply(ctx context.Context, repo Repository) (bool, error) {
	if len(repo.URLs) != 1 {
		return false, fmt.Errorf("repository %q needs exactly one uri", repo.Name)
	}
	if err := ValidateRepoURL(repo.URLs[0]); err != nil {
		return false, err
	}
	if repo.Suite == "" {
		return false, fmt.Errorf("repository %q needs a suite", repo.Name)
	}

	existing, found, err := GetRepo(ctx, b, repo.Name)
	if err != nil {
		return false, err
	}
	if found {
		if existing.Options["format"] == "deb822" {
			return false, fmt.Errorf("repository %q is in the deb822 format, which whoctl reads but does not rewrite: edit %s by hand", repo.Name, existing.File)
		}
		if existing.Options["extraEntries"] == "true" {
			return false, fmt.Errorf("%s holds more than one source line, which whoctl does not model: edit it by hand", existing.File)
		}
	}

	path := filepath.Join(b.ConfigPath(), repo.Name+".list")
	if found {
		path = existing.File
	}
	want := renderAptLine(repo)

	lines, _, err := readLines(path)
	if err != nil {
		return false, err
	}
	for i, line := range lines {
		if _, _, ok := parseAptLine(line); !ok {
			continue
		}
		if line == want {
			return false, nil
		}
		lines[i] = want
		return true, writeFile(path, joinLines(lines), 0o644)
	}
	return true, writeFile(path, joinLines(append(lines, want)), 0o644)
}

func renderAptLine(repo Repository) string {
	types := "deb"
	if len(repo.Types) > 0 {
		types = repo.Types[0]
	}
	parts := []string{types}
	if len(repo.Options) > 0 {
		var opts []string
		for _, k := range sortedKeys(repo.Options) {
			// Bookkeeping markers describe the file, not the source line.
			if k == "format" || k == "extraEntries" {
				continue
			}
			opts = append(opts, k+"="+repo.Options[k])
		}
		if len(opts) > 0 {
			parts = append(parts, "["+strings.Join(opts, " ")+"]")
		}
	}
	parts = append(parts, repo.URLs[0], repo.Suite)
	parts = append(parts, repo.Components...)

	line := strings.Join(parts, " ")
	if !repo.Enabled {
		line = "# " + line
	}
	return line
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func (b *aptRepos) Delete(ctx context.Context, name string) error {
	existing, found, err := GetRepo(ctx, b, name)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	// The main sources.list is the distribution's own and is never removed;
	// its entry is emptied instead.
	if existing.File == b.mainList() {
		return fmt.Errorf("%s is the distribution's own source list and is not deleted by whoctl", existing.File)
	}
	return os.Remove(existing.File)
}
