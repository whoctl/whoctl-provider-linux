// Package nameserver manages the nameserver lines of /etc/resolv.conf.
package nameserver

import (
	"context"
	"fmt"
	"net"
	"slices"

	"github.com/whoctl/whoctl-provider-linux/resources/resolver/resolvconf"
	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
)

// NameserverSpec is the desired state of a DNS resolver entry in
// /etc/resolv.conf. The object name is the address itself.
type NameserverSpec struct {
	Priority *int `yaml:"priority,omitempty" json:"priority,omitempty" doc:"1-based position in /etc/resolv.conf. Resolvers are tried in order, so it decides which one answers first. Omitted, an existing entry keeps its place and a new one is appended." docExample:"1"`
}

// NameserverStatus is the observed state of a resolver entry.
type NameserverStatus struct {
	Address   string `yaml:"address" json:"address" doc:"The resolver address, same as metadata.name."`
	Family    string `yaml:"family" json:"family" doc:"IPv4 or IPv6."`
	Priority  int    `yaml:"priority" json:"priority" doc:"1-based position among the nameserver lines."`
	Effective bool   `yaml:"effective" json:"effective" doc:"False for entries past MAXNS, which the C library never reaches."`
	File      string `yaml:"file" json:"file" doc:"The file the entry was read from." docExample:"/etc/resolv.conf"`
	// While it is set, whoctl refuses to write: the daemon would revert us.
	ManagedBy string `yaml:"managedBy,omitempty" json:"managedBy,omitempty" doc:"The resolver daemon that owns the file, when there is one." docExample:"systemd-resolved"`
}

// Handler serves the kind.
type Handler struct{ p *provider.Provider }

// New builds the handler.
func New(p *provider.Provider) core.Handler { return &Handler{p: p} }

func (h *Handler) Type() core.ResourceType {
	return provider.ResourceType(core.ResourceType{
		Kind:        "Nameserver",
		Plural:      "nameservers",
		Singular:    "nameserver",
		ShortNames:  []string{"ns"},
		Description: "A DNS resolver address in /etc/resolv.conf, in the order the system tries them.",
		Columns: []core.Column{
			{Name: "NAME", Path: "metadata.name"},
			{Name: "PRIORITY", Path: "status.priority"},
			{Name: "FAMILY", Path: "status.family"},
			{Name: "EFFECTIVE", Path: "status.effective"},
			{Name: "FILE", Wide: true, Path: "status.file"},
			{Name: "MANAGED-BY", Wide: true, Path: "status.managedBy"},
		},
	})
}

func (h *Handler) NewSpec() any { return &NameserverSpec{} }

func (h *Handler) NewStatus() any { return &NameserverStatus{} }

func (h *Handler) List(ctx context.Context) ([]core.Object, error) {
	conf, err := resolvconf.Load(h.p.Root)
	if err != nil {
		return nil, err
	}
	addrs := conf.Nameservers()
	out := make([]core.Object, 0, len(addrs))
	for i, addr := range addrs {
		out = append(out, h.build(conf, addr, i))
	}
	return out, nil
}

func (h *Handler) Get(ctx context.Context, name string) (core.Object, error) {
	conf, err := resolvconf.Load(h.p.Root)
	if err != nil {
		return core.Object{}, err
	}
	addrs := conf.Nameservers()
	i := slices.Index(addrs, name)
	if i < 0 {
		return core.Object{}, core.NotFound("nameserver", name)
	}
	return h.build(conf, name, i), nil
}

func (h *Handler) Apply(ctx context.Context, obj core.Object) (core.Result, error) {
	spec, ok := obj.Spec.(*NameserverSpec)
	if !ok || spec == nil {
		return core.Result{}, fmt.Errorf("nameserver %q: missing or invalid spec", obj.Metadata.Name)
	}
	address := obj.Metadata.Name
	if net.ParseIP(address) == nil {
		return core.Result{}, fmt.Errorf("nameserver %q: metadata.name must be an IP address", address)
	}

	conf, err := resolvconf.Load(h.p.Root)
	if err != nil {
		return core.Result{}, err
	}
	addrs := conf.Nameservers()
	current := slices.Index(addrs, address)

	// Already present at the requested position: nothing to do.
	if current >= 0 && (spec.Priority == nil || *spec.Priority == current+1) {
		return core.Result{Action: core.ActionUnchanged, Object: h.build(conf, address, current)}, nil
	}

	updated := slices.DeleteFunc(slices.Clone(addrs), func(a string) bool { return a == address })
	pos := len(updated)
	if spec.Priority != nil {
		pos = min(max(*spec.Priority-1, 0), len(updated))
	}
	updated = slices.Insert(updated, pos, address)

	var diff []string
	action := core.ActionConfigured
	if current < 0 {
		action = core.ActionCreated
		diff = append(diff, fmt.Sprintf("added %s at position %d", address, pos+1))
	} else {
		diff = append(diff, fmt.Sprintf("moved %s from position %d to %d", address, current+1, pos+1))
	}
	if pos >= resolvconf.MaxNameservers {
		diff = append(diff, fmt.Sprintf("warning: position %d is past MAXNS (%d), the resolver will ignore it", pos+1, resolvconf.MaxNameservers))
	}

	conf.SetNameservers(updated)
	if h.p.Runner.Mutate("rewrite " + conf.FilePath()) {
		if err := conf.Save(); err != nil {
			return core.Result{}, err
		}
		return core.Result{Action: action, Object: h.build(conf, address, pos), Diff: diff}, nil
	}
	return core.Result{Action: action, Object: obj, Diff: diff}, nil
}

func (h *Handler) Delete(ctx context.Context, name string) error {
	conf, err := resolvconf.Load(h.p.Root)
	if err != nil {
		return err
	}
	addrs := conf.Nameservers()
	if !slices.Contains(addrs, name) {
		return core.NotFound("nameserver", name)
	}

	conf.SetNameservers(slices.DeleteFunc(slices.Clone(addrs), func(a string) bool { return a == name }))
	if !h.p.Runner.Mutate("rewrite " + conf.FilePath()) {
		return nil
	}
	return conf.Save()
}

func (h *Handler) build(conf *resolvconf.Conf, address string, index int) core.Object {
	priority := index + 1
	t := h.Type()
	return core.Object{
		APIVersion: t.APIVersion(),
		Kind:       t.Kind,
		Metadata:   core.Metadata{Name: address},
		Spec:       &NameserverSpec{Priority: &priority},
		Status: &NameserverStatus{
			Address:   address,
			Family:    ipFamily(address),
			Priority:  priority,
			Effective: index < resolvconf.MaxNameservers,
			File:      conf.FilePath(),
			ManagedBy: conf.ManagedBy(),
		},
	}
}

func nameserverStatus(o core.Object) *NameserverStatus {
	if s, ok := o.Status.(*NameserverStatus); ok && s != nil {
		return s
	}
	return &NameserverStatus{}
}

func ipFamily(address string) string {
	ip := net.ParseIP(address)
	switch {
	case ip == nil:
		return "unknown"
	case ip.To4() != nil:
		return "IPv4"
	default:
		return "IPv6"
	}
}
