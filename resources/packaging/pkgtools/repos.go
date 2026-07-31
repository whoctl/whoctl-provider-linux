package pkgtools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Repository is the observed state of a package source.
//
// It is a superset: no manager uses every field, and this type is deliberately
// not what a manifest sees. Each repository kind publishes its own spec holding
// only the fields its manager actually has, so the documentation cannot promise
// a `components` list to dnf or a `gpgkey` to apk. This struct is the plumbing
// underneath that, where one interface is worth more than four.
type Repository struct {
	Name    string
	Enabled bool
	// File is where the entry was read from, which is also where a change is
	// written back.
	File string
	URLs []string

	// apk: an optional tag, as in "@edge https://…", which lets a package be
	// requested from that repository specifically.
	Tag string

	// apt
	Types      []string
	Suite      string
	Components []string
	Options    map[string]string

	// dnf
	DisplayName string
	Metalink    string
	Mirrorlist  string
	GPGCheck    *bool
	GPGKey      string

	// pacman
	Include  string
	SigLevel string
}

// RepoBackend is the set of repository operations, one implementation per
// package manager.
//
// Unlike packages, repositories are configuration files that no native tool
// owns end to end: there is no `apk repo add`, and `dnf config-manager` is a
// plugin that is not always installed. So these read and rewrite the files
// directly, in the same spirit as the resolv.conf handling — every line the
// model does not cover is preserved verbatim.
type RepoBackend interface {
	Name() string
	// ConfigPath is the file or directory the entries live in, named in errors.
	ConfigPath() string

	List(ctx context.Context) ([]Repository, error)
	// Apply creates or reconciles one entry and reports whether anything
	// changed.
	Apply(ctx context.Context, repo Repository) (bool, error)
	Delete(ctx context.Context, name string) error
}

// AllRepos returns every repository backend, in the same order as All.
func AllRepos(opts Options) []RepoBackend {
	return []RepoBackend{
		&apkRepos{opts: opts},
		&aptRepos{opts: opts},
		&dnfRepos{opts: opts},
		&pacmanRepos{opts: opts},
	}
}

// FindRepos returns the repository backend for a manager.
func FindRepos(opts Options, name string) (RepoBackend, error) {
	for _, b := range AllRepos(opts) {
		if b.Name() == name {
			return b, nil
		}
	}
	return nil, fmt.Errorf("unknown package manager %q", name)
}

// GetRepo returns one repository by name.
func GetRepo(ctx context.Context, b RepoBackend, name string) (Repository, bool, error) {
	repos, err := b.List(ctx)
	if err != nil {
		return Repository{}, false, err
	}
	for _, r := range repos {
		if r.Name == name {
			return r, true, nil
		}
	}
	return Repository{}, false, nil
}

// writeFile replaces a configuration file's contents.
//
// The rename dance is the same one resolvconf does and for the same reason: a
// rename is atomic, so a crash halfway through cannot leave a half-written
// sources file that stops the machine from installing anything. Inside a
// container the target can be a bind mount, where rename fails with EBUSY, so
// the in-place write is the fallback rather than the default.
func writeFile(path, content string, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".whoctl-*")
	if err != nil {
		return writeInPlace(path, content, perm)
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.WriteString(content)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("writing %s: %w", path, firstError(writeErr, closeErr))
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return writeInPlace(path, content, perm)
	}
	return nil
}

func writeInPlace(path, content string, perm os.FileMode) error {
	return os.WriteFile(path, []byte(content), perm)
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// sortRepos gives every backend the same stable listing order.
func sortRepos(repos []Repository) {
	sort.Slice(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })
}

// readLines returns a file's lines, and reports whether the file exists at all.
func readLines(path string) ([]string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil, true, nil
	}
	return strings.Split(text, "\n"), true, nil
}

// joinLines renders lines back into a file body with a trailing newline.
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// ValidateRepoURL rejects what would not work as a package source. It is
// deliberately loose about schemes: apk takes plain paths, pacman takes
// file:// and apt takes cdrom:, so anything without whitespace is allowed
// through to the manager, which is the real authority on what it can fetch.
func ValidateRepoURL(url string) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("repository url is empty")
	}
	if strings.ContainsAny(url, " \t") {
		return fmt.Errorf("invalid repository url %q: it cannot contain spaces", url)
	}
	return nil
}

// sortStrings keeps the sort import in one place for the file-format writers.
func sortStrings(s []string) { sort.Strings(s) }
