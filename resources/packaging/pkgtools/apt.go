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

// aptBackend drives Debian's apt.
//
// Installed packages come from dpkg's /var/lib/dpkg/status, which is a stack of
// RFC822-style records. Mutations go through apt-get rather than dpkg, so
// dependencies are resolved.
type aptBackend struct{ opts Options }

func (b *aptBackend) Name() string   { return "apt" }
func (b *aptBackend) Binary() string { return "apt-get" }

func (b *aptBackend) SupportsVersionPinning() bool { return true }

func (b *aptBackend) dbPath() string {
	return filepath.Join(rootOr(b.opts.Root), "var/lib/dpkg/status")
}

func (b *aptBackend) Installed(ctx context.Context) ([]Package, error) {
	f, err := os.Open(b.dbPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading the dpkg status file: %w", err)
	}
	defer f.Close()
	return parseDpkgStatus(f)
}

// parseDpkgStatus reads dpkg's status file.
//
// The file lists every package dpkg knows, including ones only configured or
// already removed, so the Status field decides: only "install ok installed"
// means the files are on disk. Continuation lines (leading space) belong to the
// previous field and are skipped, which keeps multi-line descriptions from
// being read as fields.
func parseDpkgStatus(r io.Reader) ([]Package, error) {
	var (
		out       []Package
		current   Package
		installed bool
	)
	flush := func() {
		if current.Name != "" && installed {
			out = append(out, current)
		}
		current, installed = Package{}, false
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "Package":
			current.Name = value
		case "Version":
			current.Version = value
		case "Architecture":
			current.Architecture = value
		case "Description":
			current.Description = value
		case "Status":
			installed = value == "install ok installed"
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading the dpkg status file: %w", err)
	}
	flush()
	return out, nil
}

// aptEnv keeps apt-get from opening a debconf dialog. Without it a package with
// questions to ask blocks on a prompt no one is there to answer.
var aptEnv = []string{"DEBIAN_FRONTEND=noninteractive"}

func (b *aptBackend) Install(ctx context.Context, name, version string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	target := name
	if version != "" {
		target = name + "=" + version
	}
	_, err := b.opts.Runner.RunWithEnv(ctx, aptEnv, "apt-get", "install", "-y", "--no-install-recommends", target)
	return err
}

func (b *aptBackend) Remove(ctx context.Context, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	// remove, not purge: configuration under /etc is the admin's, and throwing
	// it away is not something `delete` should decide on its own.
	_, err := b.opts.Runner.RunWithEnv(ctx, aptEnv, "apt-get", "remove", "-y", name)
	return err
}
