// Package pkgkind is the handler behind every package kind.

// There is one kind per manager rather than a single Package kind — see the
// provider's documentation for why — but the verbs are identical across
// managers, so the implementation is shared and only the naming differs.
package pkgkind

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/whoctl/whoctl-provider-linux/resources/packaging/pkgtools"
	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
)

// Package states, as used in spec.state and reported in status.state.
const (
	StateInstalled = "installed"
	StateAbsent    = "absent"
)

// PackageSpec is the desired state of a package.
//
// The four package kinds share this spec because the two things a manifest says
// about a package — whether it should be there, and which version — mean the
// same on every manager. What differs is the vocabulary of the values, and that
// is not something a shared type can paper over: a version string is written in
// its manager's own grammar and does not travel.
type PackageSpec struct {
	State   string `yaml:"state,omitempty" json:"state,omitempty" doc:"Whether the package should be present: installed or absent. Defaults to installed." docExample:"installed"`
	Version string `yaml:"version,omitempty" json:"version,omitempty" doc:"Exact version to hold the package at, in the manager's own format. Omitted, whoctl installs the newest version the configured repositories offer and then leaves an already-installed package alone." docExample:"8.12.1-r0"`
}

// PackageStatus is the observed state of a package.
type PackageStatus struct {
	State        string `yaml:"state" json:"state" doc:"Whether the package is installed or absent."`
	Version      string `yaml:"version,omitempty" json:"version,omitempty" doc:"The version currently installed." docExample:"8.12.1-r0"`
	Architecture string `yaml:"architecture,omitempty" json:"architecture,omitempty" doc:"The architecture the installed package was built for." docExample:"x86_64"`
	Description  string `yaml:"description,omitempty" json:"description,omitempty" doc:"One-line summary, as recorded by the package manager."`
	Origin       string `yaml:"origin,omitempty" json:"origin,omitempty" doc:"Where the package came from, when the manager records it: the source package under apk, the vendor under rpm."`
	Manager      string `yaml:"manager" json:"manager" doc:"The package manager this kind drives." docExample:"apk"`
}

// Handler implements every package kind. The verbs are identical across
// managers — the differences live behind pkgtools.Backend — so the kinds differ
// only in what they are called and which backend they carry.
// Handler serves one package kind.
type Handler struct {
	p       *provider.Provider
	backend pkgtools.Backend
	kind    string
	plural  string
	short   []string
	summary string
}

// New builds the handler for one manager. Each package kind is a directory of
// its own that calls this: the kind is not shared even though the code is.
func New(p *provider.Provider, kind, plural, manager, summary string, short ...string) core.Handler {
	backend, err := pkgtools.Find(pkgtools.OptionsFor(p.Root, p.Runner), manager)
	if err != nil {
		// Every manager is registered on every machine, so a name with no
		// backend is a programming mistake rather than a machine without it.
		panic(fmt.Sprintf("no package backend for %q: %v", manager, err))
	}
	return &Handler{p: p, backend: backend, kind: kind, plural: plural, short: short, summary: summary}
}

func (h *Handler) Type() core.ResourceType {
	return provider.ResourceType(core.ResourceType{
		Kind:        h.kind,
		Plural:      h.plural,
		Singular:    strings.TrimSuffix(h.plural, "s"),
		ShortNames:  h.short,
		Description: h.summary,
		Columns: []core.Column{
			{Name: "NAME", Path: "metadata.name"},
			{Name: "STATE", Path: "status.state"},
			{Name: "VERSION", Path: "status.version"},
			{Name: "ARCH", Wide: true, Path: "status.architecture"},
			{Name: "ORIGIN", Wide: true, Path: "status.origin"},
			{Name: "DESCRIPTION", Wide: true, Path: "status.description"},
		},
	})
}

func (h *Handler) NewSpec() any { return &PackageSpec{} }

func (h *Handler) NewStatus() any { return &PackageStatus{} }

// available guards every verb. Three of the four managers are absent from any
// given machine, and an empty list would say "nothing installed" where the truth
// is "this manager does not run here".
func (h *Handler) available() error {
	if !pkgtools.Available(h.backend) {
		return core.Unavailablef("%w", pkgtools.ErrUnavailable(h.backend))
	}
	return nil
}

func (h *Handler) List(ctx context.Context) ([]core.Object, error) {
	if err := h.available(); err != nil {
		return nil, err
	}
	pkgs, err := h.backend.Installed(ctx)
	if err != nil {
		return nil, err
	}
	// The databases come back in whatever order they are stored in — insertion
	// order for apk and dpkg, unordered for rpm — so sort by name to make two
	// runs of `get` comparable.
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Name < pkgs[j].Name })

	out := make([]core.Object, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, h.build(p, true))
	}
	return out, nil
}

func (h *Handler) Get(ctx context.Context, name string) (core.Object, error) {
	if err := h.available(); err != nil {
		return core.Object{}, err
	}
	pkg, found, err := pkgtools.Get(ctx, h.backend, name)
	if err != nil {
		return core.Object{}, err
	}
	if !found {
		return core.Object{}, core.NotFound(h.Type().Singular, name)
	}
	return h.build(pkg, true), nil
}

// Apply installs, removes or leaves the package alone.
//
// An omitted spec.version means "any version": an already-installed package is
// unchanged rather than upgraded, so running apply twice does not quietly pull
// in a new release between the two runs.
func (h *Handler) Apply(ctx context.Context, obj core.Object) (core.Result, error) {
	name := obj.Metadata.Name
	spec, ok := obj.Spec.(*PackageSpec)
	if !ok || spec == nil {
		return core.Result{}, fmt.Errorf("%s %q: missing or invalid spec", h.Type().Singular, name)
	}
	if err := h.available(); err != nil {
		return core.Result{}, err
	}
	if err := pkgtools.ValidateName(name); err != nil {
		return core.Result{}, err
	}
	state, err := normalizePackageState(spec.State)
	if err != nil {
		return core.Result{}, fmt.Errorf("%s %q: %w", h.Type().Singular, name, err)
	}
	if spec.Version != "" && !h.backend.SupportsVersionPinning() {
		return core.Result{}, fmt.Errorf("%s %q: %s cannot install a chosen version; remove spec.version", h.Type().Singular, name, h.backend.Name())
	}

	pkg, installed, err := pkgtools.Get(ctx, h.backend, name)
	if err != nil {
		return core.Result{}, err
	}

	switch {
	case state == StateAbsent && !installed:
		return core.Result{Action: core.ActionUnchanged, Object: h.build(pkgtools.Package{Name: name}, false)}, nil

	case state == StateAbsent:
		if err := h.backend.Remove(ctx, name); err != nil {
			return core.Result{}, err
		}
		return core.Result{
			Action: core.ActionConfigured,
			Object: h.build(pkgtools.Package{Name: name}, false),
			Diff:   []string{fmt.Sprintf("removed %s %s", name, pkg.Version)},
		}, nil

	case !installed:
		if err := h.backend.Install(ctx, name, spec.Version); err != nil {
			return core.Result{}, err
		}
		return h.result(ctx, name, core.ActionCreated, []string{"installed " + name + versionSuffix(spec.Version)}, obj)

	case spec.Version != "" && spec.Version != pkg.Version:
		if err := h.backend.Install(ctx, name, spec.Version); err != nil {
			return core.Result{}, err
		}
		return h.result(ctx, name, core.ActionConfigured, []string{fmt.Sprintf("version %s -> %s", pkg.Version, spec.Version)}, obj)

	default:
		return core.Result{Action: core.ActionUnchanged, Object: h.build(pkg, true)}, nil
	}
}

func (h *Handler) Delete(ctx context.Context, name string) error {
	if err := h.available(); err != nil {
		return err
	}
	_, installed, err := pkgtools.Get(ctx, h.backend, name)
	if err != nil {
		return err
	}
	if !installed {
		return core.NotFound(h.Type().Singular, name)
	}
	return h.backend.Remove(ctx, name)
}

// result re-reads the package so the returned object carries what the manager
// actually installed, which is rarely the exact string the manifest asked for.
func (h *Handler) result(ctx context.Context, name string, action core.Action, diff []string, sent core.Object) (core.Result, error) {
	if h.p.Runner.DryRun {
		return core.Result{Action: action, Object: sent, Diff: diff}, nil
	}
	obj, err := h.Get(ctx, name)
	if err != nil {
		return core.Result{}, err
	}
	return core.Result{Action: action, Object: obj, Diff: diff}, nil
}

func (h *Handler) build(pkg pkgtools.Package, installed bool) core.Object {
	t := h.Type()
	state := StateAbsent
	if installed {
		state = StateInstalled
	}

	spec := &PackageSpec{State: state}
	// The observed version goes into the spec so that `get -o yaml | apply`
	// reports unchanged — except where the manager cannot act on it. Under
	// pacman a pinned version is an error, so exporting one would produce a
	// manifest that its own machine rejects.
	if installed && h.backend.SupportsVersionPinning() {
		spec.Version = pkg.Version
	}

	return core.Object{
		APIVersion: t.APIVersion(),
		Kind:       t.Kind,
		Metadata:   core.Metadata{Name: pkg.Name},
		Spec:       spec,
		Status: &PackageStatus{
			State:        state,
			Version:      pkg.Version,
			Architecture: pkg.Architecture,
			Description:  pkg.Description,
			Origin:       pkg.Origin,
			Manager:      h.backend.Name(),
		},
	}
}

// normalizePackageState validates spec.state, accepting the words people reach
// for out of habit from other configuration tools.
func normalizePackageState(state string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", StateInstalled, "present":
		return StateInstalled, nil
	case StateAbsent, "removed", "uninstalled":
		return StateAbsent, nil
	default:
		return "", fmt.Errorf("invalid state %q: use %q or %q", state, StateInstalled, StateAbsent)
	}
}

func versionSuffix(version string) string {
	if version == "" {
		return ""
	}
	return " " + version
}
