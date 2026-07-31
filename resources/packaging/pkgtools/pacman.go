package pkgtools

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// pacmanBackend drives Arch's pacman.
//
// The local database is a directory per installed package under
// /var/lib/pacman/local, each holding a `desc` file of %FIELD% headers followed
// by their values.
type pacmanBackend struct{ opts Options }

func (b *pacmanBackend) Name() string   { return "pacman" }
func (b *pacmanBackend) Binary() string { return "pacman" }

// SupportsVersionPinning is false: pacman installs whatever version the synced
// repositories currently hold and offers no way to ask for another. Downgrading
// means reaching into the package cache by filename, which is a different
// operation with different failure modes, so the handler refuses a pinned
// version instead of quietly installing something else.
func (b *pacmanBackend) SupportsVersionPinning() bool { return false }

func (b *pacmanBackend) dbDir() string {
	return filepath.Join(rootOr(b.opts.Root), "var/lib/pacman/local")
}

func (b *pacmanBackend) Installed(ctx context.Context) ([]Package, error) {
	entries, err := os.ReadDir(b.dbDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading the pacman database: %w", err)
	}

	var out []Package
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		f, err := os.Open(filepath.Join(b.dbDir(), e.Name(), "desc"))
		if err != nil {
			// A directory without a desc file is a half-written entry, not a
			// reason to fail the whole listing.
			continue
		}
		pkg, err := parsePacmanDesc(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}
		if pkg.Name != "" {
			out = append(out, pkg)
		}
	}
	return out, nil
}

// parsePacmanDesc reads one desc file: a %FIELD% line, then the value on the
// following lines, then a blank line.
func parsePacmanDesc(r io.Reader) (Package, error) {
	var (
		pkg   Package
		field string
	)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
			field = ""
		case strings.HasPrefix(line, "%") && strings.HasSuffix(line, "%"):
			field = strings.Trim(line, "%")
		default:
			switch field {
			case "NAME":
				pkg.Name = line
			case "VERSION":
				pkg.Version = line
			case "ARCH":
				pkg.Architecture = line
			case "DESC":
				pkg.Description = line
			}
		}
	}
	return pkg, scanner.Err()
}

func (b *pacmanBackend) Install(ctx context.Context, name, version string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if version != "" {
		return fmt.Errorf("pacman cannot install a specific version: drop spec.version from %q", name)
	}
	_, err := b.opts.Runner.Run(ctx, "pacman", "-S", "--noconfirm", "--needed", name)
	return err
}

func (b *pacmanBackend) Remove(ctx context.Context, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	_, err := b.opts.Runner.Run(ctx, "pacman", "-R", "--noconfirm", name)
	return err
}
