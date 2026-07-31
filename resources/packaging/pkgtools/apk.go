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

// apkBackend drives Alpine's apk.
//
// Installed packages are read from /lib/apk/db/installed, apk's own database: a
// flat file of blank-line-separated records with one single-letter key per
// line. `apk info -v` would need the name and the version split back out of a
// single "curl-8.12.1-r0" string, which is ambiguous whenever a package name
// itself ends in something that looks like a version.
type apkBackend struct{ opts Options }

func (b *apkBackend) Name() string   { return "apk" }
func (b *apkBackend) Binary() string { return "apk" }

func (b *apkBackend) SupportsVersionPinning() bool { return true }

func (b *apkBackend) dbPath() string {
	return filepath.Join(rootOr(b.opts.Root), "lib/apk/db/installed")
}

func (b *apkBackend) Installed(ctx context.Context) ([]Package, error) {
	f, err := os.Open(b.dbPath())
	if err != nil {
		if os.IsNotExist(err) {
			// No database means nothing installed by apk, which is the truth on
			// a machine that does not run Alpine.
			return nil, nil
		}
		return nil, fmt.Errorf("reading the apk database: %w", err)
	}
	defer f.Close()
	return parseAPKDB(f)
}

// parseAPKDB reads apk's installed database. The keys used here are P (package
// name), V (version), A (architecture), T (description) and o (origin).
//
// A read error is returned rather than swallowed: a partial list would report
// an installed package as missing, and apply would then reinstall it.
func parseAPKDB(r io.Reader) ([]Package, error) {
	var (
		out     []Package
		current Package
	)
	flush := func() {
		if current.Name != "" {
			out = append(out, current)
		}
		current = Package{}
	}

	scanner := bufio.NewScanner(r)
	// Descriptions are short, but the database also holds long file lists;
	// a generous buffer keeps a single long line from aborting the scan.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch key {
		case "P":
			current.Name = value
		case "V":
			current.Version = value
		case "A":
			current.Architecture = value
		case "T":
			current.Description = value
		case "o":
			current.Origin = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading the apk database: %w", err)
	}
	flush()
	return out, nil
}

func (b *apkBackend) Install(ctx context.Context, name, version string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	target := name
	if version != "" {
		// apk pins with name=version, and the exact version has to still be in
		// a configured repository for this to resolve.
		target = name + "=" + version
	}
	_, err := b.opts.Runner.Run(ctx, "apk", "add", "--no-cache", target)
	return err
}

func (b *apkBackend) Remove(ctx context.Context, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	_, err := b.opts.Runner.Run(ctx, "apk", "del", name)
	return err
}

func rootOr(root string) string {
	if root == "" {
		return "/"
	}
	return root
}
