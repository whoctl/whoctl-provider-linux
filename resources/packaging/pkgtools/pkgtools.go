// Package pkgtools abstracts the distro package managers.
//
// It differs from usertools and servicetools on purpose. Those hide which
// backend is in use, because an account is an account whether shadow-utils or
// BusyBox created it. A package is not: the same software is `openssh-server`
// on Debian and `openssh` on Alpine, version strings follow four different
// grammars, and pacman cannot pin a version at all. A single Package kind would
// produce manifests that look portable and are not, so each manager gets its
// own kind and this package only shares what genuinely is shared.
//
// Reads follow the provider's rule and parse the manager's own database
// directly — apk, dpkg and pacman all keep theirs as flat text. rpm is the
// exception: its database is binary, so the dnf backend queries it through
// `rpm`. Writes always shell out, so the distro's own hooks and locking apply.
package pkgtools

import (
	"context"
	"fmt"
	"strings"

	"github.com/whoctl/whoctl-sdk-go/sysexec"
)

// Package is the observed state of a package, in the terms every manager shares.
type Package struct {
	Name         string
	Version      string
	Architecture string
	Description  string
	// Origin is where the package came from: the repository it was installed
	// from, when the manager records one.
	Origin string
}

// Backend is one package manager.
//
// It is deliberately narrow. Anything a single manager can do and the others
// cannot — apt holds, dnf module streams, AUR builds — belongs to that
// manager's own kind, not to a lowest common denominator that lies about what
// is portable.
type Backend interface {
	// Name identifies the manager: apk, apt, dnf or pacman.
	Name() string

	// Binary is the executable the backend drives. Reported when it is
	// missing, which is the normal case for three of the four on any machine.
	Binary() string

	// Installed lists the packages currently on the system.
	Installed(ctx context.Context) ([]Package, error)

	// Install adds a package. An empty version means the newest the configured
	// repositories offer.
	Install(ctx context.Context, name, version string) error

	// Remove deletes a package.
	Remove(ctx context.Context, name string) error

	// SupportsVersionPinning reports whether Install honours a version.
	// pacman does not, and saying so lets the handler refuse the manifest
	// instead of silently installing something else.
	SupportsVersionPinning() bool
}

// Options is what every backend needs to be built.
type Options struct {
	// Root is the filesystem root reads are resolved against. Empty means "/".
	Root string
	// Runner runs the manager. Required for install and remove.
	Runner *sysexec.Runner
}

// All returns every backend, in a stable order. They are all constructed
// regardless of what the machine runs: the resource list, and therefore the
// documentation, has to be the same everywhere, and a backend whose binary is
// absent simply reports so when used.
func All(opts Options) []Backend {
	return []Backend{
		&apkBackend{opts: opts},
		&aptBackend{opts: opts},
		&dnfBackend{opts: opts},
		&pacmanBackend{opts: opts},
	}
}

// Available reports whether the manager is actually on this machine.
func Available(b Backend) bool { return sysexec.Which(b.Binary()) != "" }

// ErrUnavailable is what every verb returns on a machine that does not run the
// manager. The message names the binary, because "apt is not here" is a fact
// about the machine and not a whoctl bug.
func ErrUnavailable(b Backend) error {
	return fmt.Errorf("%s is not available on this system: %q is not in PATH", b.Name(), b.Binary())
}

// Find returns the backend with the given name.
func Find(opts Options, name string) (Backend, error) {
	for _, b := range All(opts) {
		if b.Name() == name {
			return b, nil
		}
	}
	return nil, fmt.Errorf("unknown package manager %q", name)
}

// lookup finds one package by name in a list, which is how every backend
// implements Get: the databases are read whole anyway.
func lookup(pkgs []Package, name string) (Package, bool) {
	for _, p := range pkgs {
		if p.Name == name {
			return p, true
		}
	}
	return Package{}, false
}

// Get returns one installed package.
func Get(ctx context.Context, b Backend, name string) (Package, bool, error) {
	pkgs, err := b.Installed(ctx)
	if err != nil {
		return Package{}, false, err
	}
	p, ok := lookup(pkgs, name)
	return p, ok, nil
}

// ValidateName rejects the shell-significant characters and the version
// separators a manager would reinterpret. Package names reach argv, and a name
// carrying "=" or " " would silently become a different request.
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("package name is empty")
	}
	if strings.ContainsAny(name, " \t=<>|&;$'\"`\\") {
		return fmt.Errorf("invalid package name %q: pin a version with spec.version, not in the name", name)
	}
	return nil
}

// OptionsFor builds the backends' options from what a provider has: where to
// read from, and what to run. It is a function rather than a method on the
// provider because only the package and repository families need it.
func OptionsFor(root string, runner *sysexec.Runner) Options {
	return Options{Root: root, Runner: runner}
}
