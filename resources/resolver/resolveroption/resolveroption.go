// Package resolveroption manages the options list of /etc/resolv.conf.
package resolveroption

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/whoctl/whoctl-provider-linux/resources/resolver/resolvconf"
	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
)

// ResolverOptionSpec is the desired state of an entry in the `options`
// directive of /etc/resolv.conf. The object name is the option name, so
// `ndots:2` is an object named "ndots" whose value is "2".
type ResolverOptionSpec struct {
	Value string `yaml:"value,omitempty" json:"value,omitempty" doc:"What follows the colon in the options line. Flag options such as rotate or edns0 carry none and leave it empty." docExample:"2"`
}

// ResolverOptionStatus is the observed state of a resolver option.
type ResolverOptionStatus struct {
	Name      string `yaml:"name" json:"name" doc:"The option name, same as metadata.name."`
	Value     string `yaml:"value,omitempty" json:"value,omitempty" doc:"The value, empty for flag options."`
	Flag      bool   `yaml:"flag" json:"flag" doc:"True for options that are merely present, with no value."`
	File      string `yaml:"file" json:"file" doc:"The file the entry was read from." docExample:"/etc/resolv.conf"`
	ManagedBy string `yaml:"managedBy,omitempty" json:"managedBy,omitempty" doc:"The resolver daemon that owns the file, when there is one."`
}

// Handler serves the kind.
type Handler struct{ p *provider.Provider }

// New builds the handler.
func New(p *provider.Provider) core.Handler { return &Handler{p: p} }

func (h *Handler) Type() core.ResourceType {
	return provider.ResourceType(core.ResourceType{
		Kind:        "ResolverOption",
		Plural:      "resolveroptions",
		Singular:    "resolveroption",
		ShortNames:  []string{"resopt"},
		Description: "An entry of the options directive in /etc/resolv.conf, such as ndots:2 or rotate.",
		Columns: []core.Column{
			{Name: "NAME", Path: "metadata.name"},
			{Name: "VALUE", Path: "status.value"},
			{Name: "FILE", Wide: true, Path: "status.file"},
			{Name: "MANAGED-BY", Wide: true, Path: "status.managedBy"},
		},
	})
}

func (h *Handler) NewSpec() any { return &ResolverOptionSpec{} }

func (h *Handler) NewStatus() any { return &ResolverOptionStatus{} }

func (h *Handler) List(ctx context.Context) ([]core.Object, error) {
	conf, err := resolvconf.Load(h.p.Root)
	if err != nil {
		return nil, err
	}
	options := conf.Options()
	out := make([]core.Object, 0, len(options))
	for _, opt := range options {
		name, value := resolvconf.SplitOption(opt)
		out = append(out, h.build(conf, name, value))
	}
	return out, nil
}

func (h *Handler) Get(ctx context.Context, name string) (core.Object, error) {
	conf, err := resolvconf.Load(h.p.Root)
	if err != nil {
		return core.Object{}, err
	}
	i := indexOfOption(conf.Options(), name)
	if i < 0 {
		return core.Object{}, core.NotFound("resolveroption", name)
	}
	_, value := resolvconf.SplitOption(conf.Options()[i])
	return h.build(conf, name, value), nil
}

func (h *Handler) Apply(ctx context.Context, obj core.Object) (core.Result, error) {
	spec, ok := obj.Spec.(*ResolverOptionSpec)
	if !ok || spec == nil {
		return core.Result{}, fmt.Errorf("resolveroption %q: missing or invalid spec", obj.Metadata.Name)
	}
	name := obj.Metadata.Name
	if err := validateOption(name, spec.Value); err != nil {
		return core.Result{}, err
	}

	conf, err := resolvconf.Load(h.p.Root)
	if err != nil {
		return core.Result{}, err
	}
	options := conf.Options()
	desired := resolvconf.JoinOption(name, spec.Value)
	current := indexOfOption(options, name)

	if current >= 0 && options[current] == desired {
		return core.Result{Action: core.ActionUnchanged, Object: h.build(conf, name, spec.Value)}, nil
	}

	updated := slices.Clone(options)
	var diff []string
	action := core.ActionConfigured
	if current < 0 {
		// Options are unordered, so a new one simply goes at the end.
		updated = append(updated, desired)
		action = core.ActionCreated
		diff = append(diff, "added option "+desired)
	} else {
		diff = append(diff, fmt.Sprintf("option %s -> %s", options[current], desired))
		updated[current] = desired
	}

	conf.SetOptions(updated)
	if h.p.Runner.Mutate("rewrite " + conf.FilePath()) {
		if err := conf.Save(); err != nil {
			return core.Result{}, err
		}
		return core.Result{Action: action, Object: h.build(conf, name, spec.Value), Diff: diff}, nil
	}
	return core.Result{Action: action, Object: obj, Diff: diff}, nil
}

func (h *Handler) Delete(ctx context.Context, name string) error {
	conf, err := resolvconf.Load(h.p.Root)
	if err != nil {
		return err
	}
	options := conf.Options()
	i := indexOfOption(options, name)
	if i < 0 {
		return core.NotFound("resolveroption", name)
	}

	conf.SetOptions(slices.Delete(slices.Clone(options), i, i+1))
	if !h.p.Runner.Mutate("rewrite " + conf.FilePath()) {
		return nil
	}
	return conf.Save()
}

func (h *Handler) build(conf *resolvconf.Conf, name, value string) core.Object {
	t := h.Type()
	return core.Object{
		APIVersion: t.APIVersion(),
		Kind:       t.Kind,
		Metadata:   core.Metadata{Name: name},
		Spec:       &ResolverOptionSpec{Value: value},
		Status: &ResolverOptionStatus{
			Name:      name,
			Value:     value,
			Flag:      value == "",
			File:      conf.FilePath(),
			ManagedBy: conf.ManagedBy(),
		},
	}
}

// indexOfOption finds an option by name, ignoring whatever value it carries.
func indexOfOption(options []string, name string) int {
	for i, opt := range options {
		if optName, _ := resolvconf.SplitOption(opt); optName == name {
			return i
		}
	}
	return -1
}

func resolverOptionStatus(o core.Object) *ResolverOptionStatus {
	if s, ok := o.Status.(*ResolverOptionStatus); ok && s != nil {
		return s
	}
	return &ResolverOptionStatus{}
}

// validateOption keeps the `options` line parseable. The directive is
// space-separated with a colon between name and value, so neither part may
// contain either character.
func validateOption(name, value string) error {
	switch {
	case name == "":
		return fmt.Errorf("metadata.name is required")
	case strings.ContainsAny(name, " \t\n:"):
		return fmt.Errorf("invalid option %q: the name cannot contain spaces or ':' (put the value in spec.value)", name)
	case strings.ContainsAny(value, " \t\n:"):
		return fmt.Errorf("invalid value %q for option %q: cannot contain spaces or ':'", value, name)
	}
	return nil
}
