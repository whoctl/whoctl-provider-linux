// Package group manages local groups and their membership.
package group

import (
	"context"
	"fmt"
	"sort"

	"github.com/whoctl/whoctl-provider-linux/resources/account/etcfiles"
	"github.com/whoctl/whoctl-provider-linux/resources/account/usertools"
	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
	"github.com/whoctl/whoctl-provider-linux/resources/account"
)

// GroupSpec is the desired state of a local group.
type GroupSpec struct {
	GID *int `yaml:"gid,omitempty" json:"gid,omitempty" doc:"Numeric group ID. The system allocates one when omitted." docExample:"4000"`
	// Users whose *primary* group this is are deliberately out of scope here:
	// they are a property of the user, not of the group.
	Members []string `yaml:"members,omitempty" json:"members,omitempty" doc:"Supplementary members listed in /etc/group. Omitting the field leaves them alone, an empty list clears them." docExample:"[alice, bob]"`
	System  bool     `yaml:"system,omitempty" json:"system,omitempty" doc:"Allocate the GID outside the regular range." docFlags:"createOnly"`
}

// GroupStatus is the observed state of a local group.
type GroupStatus struct {
	GID            int      `yaml:"gid" json:"gid" doc:"Numeric group ID."`
	Members        []string `yaml:"members,omitempty" json:"members,omitempty" doc:"Supplementary members listed in /etc/group."`
	PrimaryMembers []string `yaml:"primaryMembers,omitempty" json:"primaryMembers,omitempty" doc:"Users whose primary group this is. They are not managed through spec.members and they block deletion."`
	System         bool     `yaml:"system" json:"system" doc:"Whether the GID falls outside the regular range declared in /etc/login.defs."`
}

// Handler serves the kind.
type Handler struct{ p *provider.Provider }

// New builds the handler.
func New(p *provider.Provider) core.Handler { return &Handler{p: p} }

func (h *Handler) Type() core.ResourceType {
	return provider.ResourceType(core.ResourceType{
		Kind:        "Group",
		Plural:      "groups",
		Singular:    "group",
		ShortNames:  []string{"grp"},
		Description: "A local group, as recorded in /etc/group.",
		Columns: []core.Column{
			{Name: "NAME", Path: "metadata.name"},
			{Name: "GID", Path: "status.gid"},
			{Name: "MEMBERS", Path: "status.members"},
			{Name: "PRIMARY-MEMBERS", Wide: true, Path: "status.primaryMembers"},
			{Name: "SYSTEM", Wide: true, Path: "status.system"},
		},
	})
}

func (h *Handler) NewSpec() any { return &GroupSpec{} }

func (h *Handler) NewStatus() any { return &GroupStatus{} }

func (h *Handler) List(ctx context.Context) ([]core.Object, error) {
	snap, err := account.Read(h.p)
	if err != nil {
		return nil, err
	}
	out := make([]core.Object, 0, len(snap.Groups))
	for _, g := range snap.Groups {
		out = append(out, h.build(g, snap))
	}
	sort.Slice(out, func(i, j int) bool {
		return groupStatus(out[i]).GID < groupStatus(out[j]).GID
	})
	return out, nil
}

func (h *Handler) Get(ctx context.Context, name string) (core.Object, error) {
	snap, err := account.Read(h.p)
	if err != nil {
		return core.Object{}, err
	}
	g, ok := snap.Group(name)
	if !ok {
		return core.Object{}, core.NotFound("group", name)
	}
	return h.build(g, snap), nil
}

func (h *Handler) Apply(ctx context.Context, obj core.Object) (core.Result, error) {
	spec, ok := obj.Spec.(*GroupSpec)
	if !ok || spec == nil {
		return core.Result{}, fmt.Errorf("group %q: missing or invalid spec", obj.Metadata.Name)
	}
	name := obj.Metadata.Name
	if err := account.ValidateName(name); err != nil {
		return core.Result{}, err
	}
	tools, err := account.Toolset(h.p)
	if err != nil {
		return core.Result{}, err
	}
	snap, err := account.Read(h.p)
	if err != nil {
		return core.Result{}, err
	}

	current, exists := snap.Group(name)
	if !exists {
		req := usertools.CreateGroup{Name: name, GID: spec.GID, System: spec.System}
		if err := tools.CreateGroup(ctx, req); err != nil {
			return core.Result{}, err
		}
		for _, m := range spec.Members {
			if err := tools.AddUserToGroup(ctx, m, name); err != nil {
				return core.Result{}, err
			}
		}
		obj, err := h.reload(ctx, name, obj)
		return core.Result{Action: core.ActionCreated, Object: obj, Diff: []string{"created group " + name}}, err
	}

	var diff []string
	if spec.GID != nil && *spec.GID != current.GID {
		if err := tools.SetGroupGID(ctx, name, *spec.GID); err != nil {
			return core.Result{}, err
		}
		diff = append(diff, fmt.Sprintf("gid %d -> %d", current.GID, *spec.GID))
	}
	// A nil Members means "not managed here"; an empty (but present) list
	// clears the membership.
	if spec.Members != nil {
		add, remove := account.DiffSets(current.Members, spec.Members)
		for _, m := range add {
			if err := tools.AddUserToGroup(ctx, m, name); err != nil {
				return core.Result{}, err
			}
			diff = append(diff, "added member "+m)
		}
		for _, m := range remove {
			if err := tools.RemoveUserFromGroup(ctx, m, name); err != nil {
				return core.Result{}, err
			}
			diff = append(diff, "removed member "+m)
		}
	}

	action := core.ActionUnchanged
	if len(diff) > 0 {
		action = core.ActionConfigured
	}
	updated, err := h.reload(ctx, name, obj)
	return core.Result{Action: action, Object: updated, Diff: diff}, err
}

func (h *Handler) Delete(ctx context.Context, name string) error {
	tools, err := account.Toolset(h.p)
	if err != nil {
		return err
	}
	snap, err := account.Read(h.p)
	if err != nil {
		return err
	}
	g, found := snap.Group(name)
	if !found {
		return core.NotFound("group", name)
	}
	// The native tools refuse to remove a group that is somebody's primary
	// group, but their error message does not say who; this one does.
	if users := snap.PrimaryMembers(g.GID); len(users) > 0 {
		return fmt.Errorf("group %q is the primary group of %s: remove those users first", name, account.JoinOrDash(users))
	}
	return tools.DeleteGroup(ctx, name)
}

func (h *Handler) reload(ctx context.Context, name string, sent core.Object) (core.Object, error) {
	if h.p.Runner.DryRun {
		return sent, nil
	}
	return h.Get(ctx, name)
}

func (h *Handler) build(g etcfiles.Group, snap *account.Snapshot) core.Object {
	// GID_MIN and UID_MIN are the same value on every distro that ships
	// login.defs, so the UID range doubles as the system-group heuristic.
	system := snap.UIDRange.IsSystem(g.GID)
	gid := g.GID

	t := h.Type()
	return core.Object{
		APIVersion: t.APIVersion(),
		Kind:       t.Kind,
		Metadata:   core.Metadata{Name: g.Name},
		Spec: &GroupSpec{
			GID:     &gid,
			Members: g.Members,
			System:  system,
		},
		Status: &GroupStatus{
			GID:            g.GID,
			Members:        g.Members,
			PrimaryMembers: snap.PrimaryMembers(g.GID),
			System:         system,
		},
	}
}

func groupStatus(o core.Object) *GroupStatus {
	if s, ok := o.Status.(*GroupStatus); ok && s != nil {
		return s
	}
	return &GroupStatus{}
}
