// Package linux is whoctl's first provider: it manages the local machine's
// own resources (users, groups, and later Services, nameservers and hosts).
//
// Reads come from the system Files directly; writes always go through the
// native tooling, so distro-specific behaviour (skeleton Files, locking,
// PAM hooks) is preserved.
// Package provider is the state every linux resource works from: the parsed
// /etc files, the command runner, and the lazily detected tooling.
//
// It exists because each kind is its own package under resources/, and they all
// need the same handful of things. What is shared lives here rather than inside
// one of its users, which is how a dependency gets forgotten.
package provider

import (
	"github.com/whoctl/whoctl-sdk-go/core"
	"github.com/whoctl/whoctl-sdk-go/sysexec"
)

// API group and version served by this provider.
const (
	Group   = "linux.whoctl.io"
	Version = "v1alpha1"
)

// Options configures the provider.
type Options struct {
	// Root is the filesystem root used for reads. Empty means "/". It exists
	// so tests can point at a fixture tree instead of the real machine.
	Root string
	// Runner runs the native tools. Required for any mutating verb.
	Runner *sysexec.Runner
}

// Provider is what every kind here works from, and it is deliberately this
// small: a filesystem root to read under, and a runner to write through.
//
// Anything else a kind needs belongs to the family that needs it — the parsed
// /etc to the account kinds, the init system to Service, the package backends
// to the two package families. State shared by one user is state in the wrong
// place, and keeping this type small is what makes that visible.
type Provider struct {
	// Root is what reads are relative to. Empty means "/".
	Root string
	// Runner runs the native tools, and is what --dry-run and -v act on.
	Runner *sysexec.Runner
}

// New builds the linux provider.
func New(opts Options) *Provider {
	runner := opts.Runner
	if runner == nil {
		runner = &sysexec.Runner{}
	}
	return &Provider{Root: opts.Root, Runner: runner}
}

// Name implements core.Provider. It is the prefix every kind here is addressed
// by: `whoctl get linux/users`.
func (p *Provider) Name() string { return "linux" }

// Aliases implements core.Aliaser. "nix" is three characters shorter and the
// prefix is now typed on every command, so `nix/usr` earns its place next to
// `linux/users`.
func (p *Provider) Aliases() []string { return []string{"nix"} }

// ResourceType fills in the group and version shared by every kind here.
func ResourceType(t core.ResourceType) core.ResourceType {
	t.Group = Group
	t.Version = Version
	return t
}
