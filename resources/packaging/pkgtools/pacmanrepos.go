package pkgtools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// pacmanRepos manages the repository sections of /etc/pacman.conf.
//
// pacman keeps its repositories in the same file as its own settings, so this
// backend has to be careful about what it is not looking at: the file opens
// with an [options] section that configures pacman itself, and rewriting it as
// if it were a repository would break the machine. Everything outside a
// repository section — [options], comments, blank lines — is copied through
// untouched.
//
// A repository is a `[name]` section holding either Server lines or an Include
// pointing at a mirrorlist, which is how Arch's own default configuration is
// written.
type pacmanRepos struct{ opts Options }

func (b *pacmanRepos) Name() string { return "pacman" }

func (b *pacmanRepos) ConfigPath() string {
	return filepath.Join(rootOr(b.opts.Root), "etc/pacman.conf")
}

// optionsSection is pacman's own configuration, not a repository.
const optionsSection = "options"

func (b *pacmanRepos) List(ctx context.Context) ([]Repository, error) {
	lines, _, err := readLines(b.ConfigPath())
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", b.ConfigPath(), err)
	}
	var out []Repository
	for _, s := range parsePacmanConf(lines) {
		if strings.EqualFold(s.name, optionsSection) {
			continue
		}
		out = append(out, s.toRepository(b.ConfigPath()))
	}
	sortRepos(out)
	return out, nil
}

// pacmanSection is one [name] block. Body holds every line as written, so a
// section whoctl only reads is put back exactly as it was found.
type pacmanSection struct {
	name    string
	header  string
	body    []string
	enabled bool
	// leading is what stood between the previous section and this header.
	leading []string
}

// parsePacmanConf splits the file into sections.
//
// A commented-out header — "#[multilib]" — is how the shipped pacman.conf
// carries a repository that is available but off, so it is read as a disabled
// section rather than as a comment. Its body lines are commented too and stay
// that way until the section is enabled.
func parsePacmanConf(lines []string) []pacmanSection {
	var (
		sections []pacmanSection
		current  *pacmanSection
		pending  []string
	)
	flush := func() {
		if current != nil {
			sections = append(sections, *current)
			current = nil
		}
	}
	for _, line := range lines {
		name, enabled, ok := pacmanHeader(line)
		if ok {
			flush()
			current = &pacmanSection{name: name, header: line, enabled: enabled, leading: pending}
			pending = nil
			continue
		}
		if current == nil {
			pending = append(pending, line)
			continue
		}
		current.body = append(current.body, line)
	}
	flush()
	if len(pending) > 0 {
		// Trailing comments at the end of the file belong to nothing; they are
		// preserved as a nameless tail section.
		sections = append(sections, pacmanSection{body: pending})
	}
	return sections
}

// pacmanHeader recognises "[name]" and the disabled "#[name]".
func pacmanHeader(line string) (name string, enabled bool, ok bool) {
	text := strings.TrimSpace(line)
	enabled = true
	if strings.HasPrefix(text, "#") {
		enabled = false
		text = strings.TrimSpace(strings.TrimLeft(text, "#"))
	}
	if !strings.HasPrefix(text, "[") || !strings.HasSuffix(text, "]") {
		return "", false, false
	}
	name = strings.TrimSpace(strings.Trim(text, "[]"))
	if name == "" || strings.ContainsAny(name, " \t") {
		return "", false, false
	}
	return name, enabled, true
}

func (s pacmanSection) toRepository(path string) Repository {
	repo := Repository{Name: s.name, Enabled: s.enabled, File: path}
	for _, line := range s.body {
		key, value, ok := pacmanEntry(line)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "server":
			repo.URLs = append(repo.URLs, value)
		case "include":
			repo.Include = value
		case "siglevel":
			repo.SigLevel = value
		}
	}
	return repo
}

// pacmanEntry reads a "Key = value" line, seeing through the comment marker a
// disabled section's lines carry.
func pacmanEntry(line string) (key, value string, ok bool) {
	text := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
	key, value, ok = strings.Cut(text, "=")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), true
}

func (b *pacmanRepos) Apply(ctx context.Context, repo Repository) (bool, error) {
	if repo.Name == "" {
		return false, fmt.Errorf("repository name is empty")
	}
	if strings.EqualFold(repo.Name, optionsSection) {
		return false, fmt.Errorf("[options] is pacman's own configuration, not a repository")
	}
	for _, url := range repo.URLs {
		if err := ValidateRepoURL(url); err != nil {
			return false, err
		}
	}
	if len(repo.URLs) == 0 && repo.Include == "" {
		return false, fmt.Errorf("repository %q needs either a server or an include", repo.Name)
	}

	lines, _, err := readLines(b.ConfigPath())
	if err != nil {
		return false, err
	}
	sections := parsePacmanConf(lines)
	rendered := renderPacmanSection(repo)

	for i, s := range sections {
		if s.name != repo.Name {
			continue
		}
		if s.header == rendered.header && equalLines(s.body, rendered.body) {
			return false, nil
		}
		rendered.leading = s.leading
		sections[i] = rendered
		return true, b.save(sections)
	}
	rendered.leading = []string{""}
	return true, b.save(append(sections, rendered))
}

func renderPacmanSection(repo Repository) pacmanSection {
	prefix := ""
	if !repo.Enabled {
		prefix = "#"
	}
	s := pacmanSection{name: repo.Name, enabled: repo.Enabled, header: prefix + "[" + repo.Name + "]"}
	if repo.SigLevel != "" {
		s.body = append(s.body, prefix+"SigLevel = "+repo.SigLevel)
	}
	if repo.Include != "" {
		s.body = append(s.body, prefix+"Include = "+repo.Include)
	}
	for _, url := range repo.URLs {
		s.body = append(s.body, prefix+"Server = "+url)
	}
	return s
}

func (b *pacmanRepos) Delete(ctx context.Context, name string) error {
	lines, _, err := readLines(b.ConfigPath())
	if err != nil {
		return err
	}
	sections := parsePacmanConf(lines)
	kept := make([]pacmanSection, 0, len(sections))
	for _, s := range sections {
		if s.name == name {
			continue
		}
		kept = append(kept, s)
	}
	return b.save(kept)
}

func (b *pacmanRepos) save(sections []pacmanSection) error {
	var out []string
	for _, s := range sections {
		out = append(out, s.leading...)
		if s.header != "" {
			out = append(out, s.header)
		}
		out = append(out, s.body...)
	}
	return writeFile(b.ConfigPath(), joinLines(out), 0o644)
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}
