package pkgtools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// apkRepos manages /etc/apk/repositories.
//
// The file is the simplest of the four: one repository per line, nothing else.
// A line may carry a leading @tag, which lets `apk add pkg@tag` pull that
// package from that repository specifically. A commented-out line is how apk
// users disable a repository without losing the URL, so that is what
// `enabled: false` writes.
type apkRepos struct{ opts Options }

func (b *apkRepos) Name() string { return "apk" }

func (b *apkRepos) ConfigPath() string {
	return filepath.Join(rootOr(b.opts.Root), "etc/apk/repositories")
}

func (b *apkRepos) List(ctx context.Context) ([]Repository, error) {
	lines, _, err := readLines(b.ConfigPath())
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", b.ConfigPath(), err)
	}
	var out []Repository
	for _, line := range lines {
		if repo, ok := parseAPKRepoLine(line); ok {
			repo.File = b.ConfigPath()
			out = append(out, repo)
		}
	}
	sortRepos(out)
	return out, nil
}

// parseAPKRepoLine reads one line. A disabled repository is a commented-out
// one, so a comment is only skipped when what follows it does not look like a
// URL — otherwise "#https://..." would be invisible instead of disabled.
func parseAPKRepoLine(line string) (Repository, bool) {
	text := strings.TrimSpace(line)
	if text == "" {
		return Repository{}, false
	}

	enabled := true
	if strings.HasPrefix(text, "#") {
		enabled = false
		text = strings.TrimSpace(strings.TrimLeft(text, "#"))
	}
	if text == "" {
		return Repository{}, false
	}

	tag := ""
	if strings.HasPrefix(text, "@") {
		parts := strings.Fields(text)
		if len(parts) < 2 {
			return Repository{}, false
		}
		tag = strings.TrimPrefix(parts[0], "@")
		text = parts[1]
	}
	if strings.ContainsAny(text, " \t") || !looksLikeAPKSource(text) {
		return Repository{}, false
	}
	return Repository{Name: text, URLs: []string{text}, Enabled: enabled, Tag: tag}, true
}

// looksLikeAPKSource keeps ordinary prose comments out of the listing. apk
// accepts remote URLs and local paths, and nothing else.
func looksLikeAPKSource(text string) bool {
	for _, scheme := range []string{"http://", "https://", "ftp://", "file://"} {
		if strings.HasPrefix(text, scheme) {
			return true
		}
	}
	return strings.HasPrefix(text, "/")
}

func (b *apkRepos) Apply(ctx context.Context, repo Repository) (bool, error) {
	if err := ValidateRepoURL(repo.Name); err != nil {
		return false, err
	}
	lines, _, err := readLines(b.ConfigPath())
	if err != nil {
		return false, err
	}

	want := renderAPKRepoLine(repo)
	for i, line := range lines {
		parsed, ok := parseAPKRepoLine(line)
		if !ok || parsed.Name != repo.Name {
			continue
		}
		if line == want {
			return false, nil
		}
		// Replaced in place, so a repository keeps its position: apk queries
		// them in file order and the first match wins.
		lines[i] = want
		return true, b.save(lines)
	}
	return true, b.save(append(lines, want))
}

func renderAPKRepoLine(repo Repository) string {
	line := repo.Name
	if repo.Tag != "" {
		line = "@" + repo.Tag + " " + line
	}
	if !repo.Enabled {
		line = "#" + line
	}
	return line
}

func (b *apkRepos) Delete(ctx context.Context, name string) error {
	lines, _, err := readLines(b.ConfigPath())
	if err != nil {
		return err
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if parsed, ok := parseAPKRepoLine(line); ok && parsed.Name == name {
			continue
		}
		out = append(out, line)
	}
	return b.save(out)
}

func (b *apkRepos) save(lines []string) error {
	return writeFile(b.ConfigPath(), joinLines(lines), 0o644)
}
