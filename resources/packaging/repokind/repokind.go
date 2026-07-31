// Package repokind is the handler behind every repository kind.

// No native tool owns these files end to end, so the provider rewrites them,
// keeping every key, comment and section the model does not cover.
package repokind

import (
	"context"
	"fmt"

	"github.com/whoctl/whoctl-provider-linux/resources/packaging/pkgtools"
	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
)

// The four repository specs.
//
// They are separate types on purpose. A repository is where a manager looks for
// packages, and the four managers disagree about what that means down to the
// shape of the data: apk has a bare URL, apt has a suite and components, dnf has
// an id with a dozen optional keys, pacman has servers or an include. One
// shared spec would have to offer every field to every kind and then document
// which ones do nothing, which is exactly the lie the per-manager kinds exist to
// avoid.

// Handler implements every repository kind. The verbs and their
// semantics are shared; the four data shapes are not, so conversion to and from
// pkgtools.Repository is supplied per kind.
// Handler serves one repository kind.
type Handler struct {
	p       *provider.Provider
	repos   pkgtools.RepoBackend
	manager pkgtools.Backend

	Config
}

// Config is everything one repository kind decides for itself. The verbs are
// identical across managers; the naming, the spec and the translation are not.
type Config struct {
	// Manager is the package manager whose repositories this kind serves.
	Manager string

	Kind string
	// plural and singular are both spelled out: deriving one from the other
	// turns "apkrepositories" into "apkrepositorie".
	Plural   string
	Singular string
	Short    []string
	Summary  string
	Columns  []core.Column

	// Spec and Status hand out the kind's own types. They are not called
	// NewSpec and NewStatus because Handler has methods by those names, and a
	// field shadowed by a method is a thing nobody should have to know about.
	Spec   func() any
	Status func() any
	// toRepo turns a manifest into the internal shape. It reports an error for
	// a spec that its manager cannot express.
	ToRepo func(name string, spec any) (pkgtools.Repository, error)
	// fromRepo turns an observation into the kind's own spec and status.
	FromRepo func(pkgtools.Repository) (any, any)
}

func (h *Handler) Type() core.ResourceType {
	return provider.ResourceType(core.ResourceType{
		Kind:        h.Kind,
		Plural:      h.Plural,
		Singular:    h.Singular,
		ShortNames:  h.Short,
		Description: h.Summary,
		Columns:     h.Columns,
	})
}

func (h *Handler) NewSpec() any { return h.Config.Spec() }

func (h *Handler) NewStatus() any { return h.Config.Status() }

// available guards every verb, the same way the package kinds do: a repository
// file belonging to a manager the machine does not run is not something whoctl
// should be editing.
func (h *Handler) available() error {
	if !pkgtools.Available(h.manager) {
		return core.Unavailablef("%w", pkgtools.ErrUnavailable(h.manager))
	}
	return nil
}

func (h *Handler) List(ctx context.Context) ([]core.Object, error) {
	if err := h.available(); err != nil {
		return nil, err
	}
	repos, err := h.repos.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]core.Object, 0, len(repos))
	for _, r := range repos {
		out = append(out, h.build(r))
	}
	return out, nil
}

func (h *Handler) Get(ctx context.Context, name string) (core.Object, error) {
	if err := h.available(); err != nil {
		return core.Object{}, err
	}
	repo, found, err := pkgtools.GetRepo(ctx, h.repos, name)
	if err != nil {
		return core.Object{}, err
	}
	if !found {
		return core.Object{}, core.NotFound(h.Type().Singular, name)
	}
	return h.build(repo), nil
}

func (h *Handler) Apply(ctx context.Context, obj core.Object) (core.Result, error) {
	name := obj.Metadata.Name
	if err := h.available(); err != nil {
		return core.Result{}, err
	}
	repo, err := h.ToRepo(name, obj.Spec)
	if err != nil {
		return core.Result{}, fmt.Errorf("%s %q: %w", h.Type().Singular, name, err)
	}

	_, existed, err := pkgtools.GetRepo(ctx, h.repos, name)
	if err != nil {
		return core.Result{}, err
	}

	// The file is rewritten by whoctl rather than by a native tool, so the
	// runner is asked rather than told: this is the same path --dry-run and -v
	// take for resolv.conf.
	if !h.p.Runner.Mutate(fmt.Sprintf("rewrite %s for %s %q", h.repos.ConfigPath(), h.Type().Singular, name)) {
		action := core.ActionCreated
		if existed {
			action = core.ActionConfigured
		}
		return core.Result{Action: action, Object: obj}, nil
	}

	changed, err := h.repos.Apply(ctx, repo)
	if err != nil {
		return core.Result{}, err
	}
	action := core.ActionUnchanged
	switch {
	case !existed:
		action = core.ActionCreated
	case changed:
		action = core.ActionConfigured
	}
	updated, err := h.Get(ctx, name)
	if err != nil {
		return core.Result{}, err
	}
	return core.Result{Action: action, Object: updated}, nil
}

func (h *Handler) Delete(ctx context.Context, name string) error {
	if err := h.available(); err != nil {
		return err
	}
	_, found, err := pkgtools.GetRepo(ctx, h.repos, name)
	if err != nil {
		return err
	}
	if !found {
		return core.NotFound(h.Type().Singular, name)
	}
	if !h.p.Runner.Mutate(fmt.Sprintf("remove %s %q from %s", h.Type().Singular, name, h.repos.ConfigPath())) {
		return nil
	}
	return h.repos.Delete(ctx, name)
}

func (h *Handler) build(repo pkgtools.Repository) core.Object {
	t := h.Type()
	spec, status := h.FromRepo(repo)
	return core.Object{
		APIVersion: t.APIVersion(),
		Kind:       t.Kind,
		Metadata:   core.Metadata{Name: repo.Name},
		Spec:       spec,
		Status:     status,
	}
}

// BoolOr reads an optional flag, falling back when the manifest left it out.
// nil is not false: a field nobody wrote is a field nobody has an opinion about.
func BoolOr(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

// BoolPtr is how an observed value becomes a spec field that round-trips.
func BoolPtr(v bool) *bool { return &v }

// First is the first of a list, or empty. A repository with no URL is a
// repository the parser could not make sense of, not a crash.
func First(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// New builds the handler for one repository kind. It resolves the two backends
// the kind needs: the one that reads its files, and the package manager whose
// presence decides whether the kind works on this machine at all.
func New(p *provider.Provider, cfg Config) core.Handler {
	opts := pkgtools.OptionsFor(p.Root, p.Runner)
	repos, err := pkgtools.FindRepos(opts, cfg.Manager)
	if err != nil {
		panic(fmt.Sprintf("no repository backend for %q: %v", cfg.Manager, err))
	}
	manager, err := pkgtools.Find(opts, cfg.Manager)
	if err != nil {
		panic(fmt.Sprintf("no package backend for %q: %v", cfg.Manager, err))
	}
	return &Handler{p: p, repos: repos, manager: manager, Config: cfg}
}
