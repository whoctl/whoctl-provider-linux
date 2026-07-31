// Package searchdomain manages the search list of /etc/resolv.conf.
package searchdomain

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/whoctl/whoctl-provider-linux/resources/resolver/resolvconf"
	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
)

// SearchDomainSpec is the desired state of an entry in the `search` directive
// of /etc/resolv.conf. The object name is the domain itself.
type SearchDomainSpec struct {
	Priority *int `yaml:"priority,omitempty" json:"priority,omitempty" doc:"1-based position in the search list. Unqualified names are tried against each domain in order, so it decides which one wins. Omitted, an existing entry keeps its place and a new one is appended." docExample:"1"`
}

// SearchDomainStatus is the observed state of a search domain.
type SearchDomainStatus struct {
	Domain    string `yaml:"domain" json:"domain" doc:"The domain, same as metadata.name."`
	Priority  int    `yaml:"priority" json:"priority" doc:"1-based position in the search list."`
	Effective bool   `yaml:"effective" json:"effective" doc:"False for entries past the resolver's limit, which are never consulted."`
	File      string `yaml:"file" json:"file" doc:"The file the entry was read from." docExample:"/etc/resolv.conf"`
	ManagedBy string `yaml:"managedBy,omitempty" json:"managedBy,omitempty" doc:"The resolver daemon that owns the file, when there is one." docExample:"NetworkManager"`
}

// Handler serves the kind.
type Handler struct{ p *provider.Provider }

// New builds the handler.
func New(p *provider.Provider) core.Handler { return &Handler{p: p} }

func (h *Handler) Type() core.ResourceType {
	return provider.ResourceType(core.ResourceType{
		Kind:        "SearchDomain",
		Plural:      "searchdomains",
		Singular:    "searchdomain",
		ShortNames:  []string{"search"},
		Description: "A domain in the search list of /etc/resolv.conf, used to complete unqualified names.",
		Columns: []core.Column{
			{Name: "NAME", Path: "metadata.name"},
			{Name: "PRIORITY", Path: "status.priority"},
			{Name: "EFFECTIVE", Path: "status.effective"},
			{Name: "FILE", Wide: true, Path: "status.file"},
			{Name: "MANAGED-BY", Wide: true, Path: "status.managedBy"},
		},
	})
}

func (h *Handler) NewSpec() any { return &SearchDomainSpec{} }

func (h *Handler) NewStatus() any { return &SearchDomainStatus{} }

func (h *Handler) List(ctx context.Context) ([]core.Object, error) {
	conf, err := resolvconf.Load(h.p.Root)
	if err != nil {
		return nil, err
	}
	domains := conf.SearchDomains()
	out := make([]core.Object, 0, len(domains))
	for i, d := range domains {
		out = append(out, h.build(conf, d, i))
	}
	return out, nil
}

func (h *Handler) Get(ctx context.Context, name string) (core.Object, error) {
	conf, err := resolvconf.Load(h.p.Root)
	if err != nil {
		return core.Object{}, err
	}
	i := slices.Index(conf.SearchDomains(), name)
	if i < 0 {
		return core.Object{}, core.NotFound("searchdomain", name)
	}
	return h.build(conf, name, i), nil
}

func (h *Handler) Apply(ctx context.Context, obj core.Object) (core.Result, error) {
	spec, ok := obj.Spec.(*SearchDomainSpec)
	if !ok || spec == nil {
		return core.Result{}, fmt.Errorf("searchdomain %q: missing or invalid spec", obj.Metadata.Name)
	}
	domain := obj.Metadata.Name
	if err := validateDomain(domain); err != nil {
		return core.Result{}, err
	}

	conf, err := resolvconf.Load(h.p.Root)
	if err != nil {
		return core.Result{}, err
	}
	domains := conf.SearchDomains()
	current := slices.Index(domains, domain)

	if current >= 0 && (spec.Priority == nil || *spec.Priority == current+1) {
		return core.Result{Action: core.ActionUnchanged, Object: h.build(conf, domain, current)}, nil
	}

	updated := slices.DeleteFunc(slices.Clone(domains), func(d string) bool { return d == domain })
	pos := len(updated)
	if spec.Priority != nil {
		pos = min(max(*spec.Priority-1, 0), len(updated))
	}
	updated = slices.Insert(updated, pos, domain)

	var diff []string
	action := core.ActionConfigured
	if current < 0 {
		action = core.ActionCreated
		diff = append(diff, fmt.Sprintf("added %s at position %d", domain, pos+1))
	} else {
		diff = append(diff, fmt.Sprintf("moved %s from position %d to %d", domain, current+1, pos+1))
	}
	if pos >= resolvconf.MaxSearchDomains {
		diff = append(diff, fmt.Sprintf("warning: position %d is past the resolver limit (%d), it will be ignored",
			pos+1, resolvconf.MaxSearchDomains))
	}

	conf.SetSearchDomains(updated)
	if h.p.Runner.Mutate("rewrite " + conf.FilePath()) {
		if err := conf.Save(); err != nil {
			return core.Result{}, err
		}
		return core.Result{Action: action, Object: h.build(conf, domain, pos), Diff: diff}, nil
	}
	return core.Result{Action: action, Object: obj, Diff: diff}, nil
}

func (h *Handler) Delete(ctx context.Context, name string) error {
	conf, err := resolvconf.Load(h.p.Root)
	if err != nil {
		return err
	}
	domains := conf.SearchDomains()
	if !slices.Contains(domains, name) {
		return core.NotFound("searchdomain", name)
	}

	conf.SetSearchDomains(slices.DeleteFunc(slices.Clone(domains), func(d string) bool { return d == name }))
	if !h.p.Runner.Mutate("rewrite " + conf.FilePath()) {
		return nil
	}
	return conf.Save()
}

func (h *Handler) build(conf *resolvconf.Conf, domain string, index int) core.Object {
	priority := index + 1
	t := h.Type()
	return core.Object{
		APIVersion: t.APIVersion(),
		Kind:       t.Kind,
		Metadata:   core.Metadata{Name: domain},
		Spec:       &SearchDomainSpec{Priority: &priority},
		Status: &SearchDomainStatus{
			Domain:    domain,
			Priority:  priority,
			Effective: index < resolvconf.MaxSearchDomains,
			File:      conf.FilePath(),
			ManagedBy: conf.ManagedBy(),
		},
	}
}

func searchDomainStatus(o core.Object) *SearchDomainStatus {
	if s, ok := o.Status.(*SearchDomainStatus); ok && s != nil {
		return s
	}
	return &SearchDomainStatus{}
}

// validateDomain rejects what cannot go on a `search` line. The directive is
// space-separated, so a value containing a space would silently become two
// domains.
func validateDomain(domain string) error {
	switch {
	case domain == "":
		return fmt.Errorf("metadata.name is required")
	case strings.ContainsAny(domain, " \t\n"):
		return fmt.Errorf("invalid domain %q: cannot contain spaces", domain)
	case strings.HasPrefix(domain, "-"):
		return fmt.Errorf("invalid domain %q: cannot start with '-'", domain)
	case len(domain) > 253:
		return fmt.Errorf("invalid domain %q: longer than 253 characters", domain)
	}
	return nil
}
