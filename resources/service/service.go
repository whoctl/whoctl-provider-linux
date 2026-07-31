// Package service manages init system services: whether they run now and
// whether they come back at boot.
package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/whoctl/whoctl-provider-linux/resources/service/servicetools"
	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
)

// Service states, as used in spec.state and reported in status.state.
const (
	StateRunning = "running"
	StateStopped = "stopped"
)

// ServiceSpec is the desired state of a service. Both fields are optional: what
// is omitted is left as it is, so a manifest can manage boot behaviour without
// touching the running state, or the other way round.
type ServiceSpec struct {
	Enabled *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty" doc:"Whether the service starts at boot."`
	State   string `yaml:"state,omitempty" json:"state,omitempty" doc:"The state right now: running or stopped." docExample:"running"`
}

// ServiceStatus is the observed state of a service.
type ServiceStatus struct {
	State       string   `yaml:"state" json:"state" doc:"Whether the service is running or stopped."`
	Enabled     bool     `yaml:"enabled" json:"enabled" doc:"Whether the service starts at boot."`
	Description string   `yaml:"description,omitempty" json:"description,omitempty" doc:"Description taken from the init script or the unit file."`
	InitSystem  string   `yaml:"initSystem" json:"initSystem" doc:"The init system behind the service: openrc or systemd."`
	Runlevels   []string `yaml:"runlevels,omitempty" json:"runlevels,omitempty" doc:"OpenRC only: the runlevels the service is enabled in."`
	// The word carries more nuance than the boolean: static and masked units
	// are both "not enabled" without being the same thing at all.
	UnitState string `yaml:"unitState,omitempty" json:"unitState,omitempty" doc:"systemd only: the raw enablement word, such as enabled, disabled, static or masked." docExample:"enabled"`
}

// Handler serves the kind.
//
// It carries its own init-system backend rather than asking the shared provider
// for one: no other kind needs it, and shared state that only one user reads is
// state in the wrong place.
type Handler struct {
	p *provider.Provider

	// Detected lazily, so `whoctl get linux/users` does not fail on a machine
	// running neither systemd nor OpenRC.
	once    sync.Once
	backend servicetools.Backend
	err     error
}

// services detects the init system on first use.
func (h *Handler) services() (servicetools.Backend, error) {
	h.once.Do(func() {
		h.backend, h.err = servicetools.Detect(h.p.Runner, h.p.Root)
	})
	return h.backend, h.err
}

// New builds the handler.
func New(p *provider.Provider) core.Handler { return &Handler{p: p} }

func (h *Handler) Type() core.ResourceType {
	return provider.ResourceType(core.ResourceType{
		Kind:        "Service",
		Plural:      "services",
		Singular:    "service",
		ShortNames:  []string{"svc"},
		Description: "An init system service: whether it runs now and whether it comes back at boot.",
		Columns: []core.Column{
			{Name: "NAME", Path: "metadata.name"},
			{Name: "STATE", Path: "status.state"},
			{Name: "ENABLED", Path: "status.enabled"},
			{Name: "INIT", Wide: true, Path: "status.initSystem"},
			{Name: "RUNLEVELS", Wide: true, Path: "status.runlevels"},
			{Name: "DESCRIPTION", Wide: true, Path: "status.description"},
		},
	})
}

func (h *Handler) NewSpec() any { return &ServiceSpec{} }

func (h *Handler) NewStatus() any { return &ServiceStatus{} }

func (h *Handler) List(ctx context.Context) ([]core.Object, error) {
	backend, err := h.services()
	if err != nil {
		return nil, err
	}
	services, err := backend.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]core.Object, 0, len(services))
	for _, s := range services {
		out = append(out, h.build(backend, s))
	}
	return out, nil
}

func (h *Handler) Get(ctx context.Context, name string) (core.Object, error) {
	backend, svc, err := h.lookup(ctx, name)
	if err != nil {
		return core.Object{}, err
	}
	return h.build(backend, svc), nil
}

// Apply reconciles enablement and running state. It never creates a service:
// what defines one is an init script or a unit file, which belongs to a package
// manager or a configuration management tool, not to whoctl.
func (h *Handler) Apply(ctx context.Context, obj core.Object) (core.Result, error) {
	spec, ok := obj.Spec.(*ServiceSpec)
	if !ok || spec == nil {
		return core.Result{}, fmt.Errorf("service %q: missing or invalid spec", obj.Metadata.Name)
	}
	desiredState, err := normalizeState(spec.State)
	if err != nil {
		return core.Result{}, fmt.Errorf("service %q: %w", obj.Metadata.Name, err)
	}

	backend, svc, err := h.lookup(ctx, obj.Metadata.Name)
	if err != nil {
		if core.IsNotFound(err) {
			return core.Result{}, core.Unsupportedf("service %q is not installed: whoctl manages the state of services, not their definitions", obj.Metadata.Name)
		}
		return core.Result{}, err
	}

	var diff []string
	// Enablement comes first: a service that is about to be started should
	// already be set to come back after a reboot.
	if spec.Enabled != nil && *spec.Enabled != svc.Enabled {
		if err := backend.SetEnabled(ctx, svc, *spec.Enabled); err != nil {
			return core.Result{}, err
		}
		diff = append(diff, fmt.Sprintf("enabled %t -> %t", svc.Enabled, *spec.Enabled))
	}
	if desiredState != "" && desiredState != stateOf(svc) {
		if err := backend.SetRunning(ctx, svc.Name, desiredState == StateRunning); err != nil {
			return core.Result{}, err
		}
		diff = append(diff, fmt.Sprintf("state %s -> %s", stateOf(svc), desiredState))
	}

	action := core.ActionUnchanged
	if len(diff) > 0 {
		action = core.ActionConfigured
	}
	updated, err := h.reload(ctx, obj.Metadata.Name, obj)
	return core.Result{Action: action, Object: updated, Diff: diff}, err
}

// Delete is deliberately unsupported: removing a unit file or an init script is
// the package manager's job, and silently stopping the service instead would be
// a surprising thing for `delete` to do.
func (h *Handler) Delete(ctx context.Context, name string) error {
	if _, _, err := h.lookup(ctx, name); err != nil {
		return err
	}
	return core.Unsupportedf("services cannot be deleted by whoctl: to turn one off, apply `state: stopped` and `enabled: false`")
}

// Restart implements core.Restarter, backing `whoctl restart service NAME`.
func (h *Handler) Restart(ctx context.Context, name string) error {
	backend, svc, err := h.lookup(ctx, name)
	if err != nil {
		return err
	}
	return backend.Restart(ctx, svc.Name)
}

func (h *Handler) lookup(ctx context.Context, name string) (servicetools.Backend, servicetools.Service, error) {
	backend, err := h.services()
	if err != nil {
		return nil, servicetools.Service{}, err
	}
	svc, found, err := backend.Get(ctx, name)
	if err != nil {
		return nil, servicetools.Service{}, err
	}
	if !found {
		return nil, servicetools.Service{}, core.NotFound("service", name)
	}
	return backend, svc, nil
}

func (h *Handler) reload(ctx context.Context, name string, sent core.Object) (core.Object, error) {
	if h.p.Runner.DryRun {
		return sent, nil
	}
	return h.Get(ctx, name)
}

func (h *Handler) build(backend servicetools.Backend, svc servicetools.Service) core.Object {
	enabled := svc.Enabled
	t := h.Type()
	return core.Object{
		APIVersion: t.APIVersion(),
		Kind:       t.Kind,
		Metadata:   core.Metadata{Name: svc.Name},
		Spec: &ServiceSpec{
			Enabled: &enabled,
			State:   stateOf(svc),
		},
		Status: &ServiceStatus{
			State:       stateOf(svc),
			Enabled:     svc.Enabled,
			Description: svc.Description,
			InitSystem:  backend.Name(),
			Runlevels:   svc.Runlevels,
			UnitState:   svc.UnitState,
		},
	}
}

func stateOf(svc servicetools.Service) string {
	if svc.Running {
		return StateRunning
	}
	return StateStopped
}

// normalizeState validates spec.state, accepting the handful of synonyms people
// reach for out of habit.
func normalizeState(state string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "":
		return "", nil
	case StateRunning, "started", "active":
		return StateRunning, nil
	case StateStopped, "inactive":
		return StateStopped, nil
	default:
		return "", fmt.Errorf("invalid state %q: use %q or %q", state, StateRunning, StateStopped)
	}
}

func serviceStatus(o core.Object) *ServiceStatus {
	if s, ok := o.Status.(*ServiceStatus); ok && s != nil {
		return s
	}
	return &ServiceStatus{}
}
