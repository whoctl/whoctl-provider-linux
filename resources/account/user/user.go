// Package user manages local login accounts.
package user

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/whoctl/whoctl-provider-linux/resources/account/etcfiles"
	"github.com/whoctl/whoctl-provider-linux/resources/account/usertools"
	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
	"github.com/whoctl/whoctl-provider-linux/resources/account"
)

// UserSpec is the desired state of a local account. Every field is optional:
// what is absent is left to the native tool's default (on create) or left
// untouched (on update).
type UserSpec struct {
	UID          *int     `yaml:"uid,omitempty" json:"uid,omitempty" doc:"Numeric user ID. The system allocates one when omitted." docExample:"4200"`
	PrimaryGroup string   `yaml:"primaryGroup,omitempty" json:"primaryGroup,omitempty" doc:"Login group of the account. It must already exist." docExample:"developers"`
	Groups       []string `yaml:"groups,omitempty" json:"groups,omitempty" doc:"Supplementary groups. Omitting the field leaves memberships alone, an empty list removes every one of them." docExample:"[wheel]"`
	Shell        string   `yaml:"shell,omitempty" json:"shell,omitempty" doc:"Login shell." docExample:"/bin/sh"`
	Home         string   `yaml:"home,omitempty" json:"home,omitempty" doc:"Home directory. Changing it on an existing account moves the contents across." docExample:"/home/alice"`
	Comment      string   `yaml:"comment,omitempty" json:"comment,omitempty" doc:"The GECOS field, usually the full name of the person." docExample:"Alice Liddell"`
	System       bool     `yaml:"system,omitempty" json:"system,omitempty" doc:"Allocate the UID outside the regular range, for accounts owned by a package rather than a person." docFlags:"createOnly"`
	// nil means "let the native tool decide", which is not the same as false.
	CreateHome *bool `yaml:"createHome,omitempty" json:"createHome,omitempty" doc:"Whether to create the home directory. Left to the native tool when omitted." docFlags:"createOnly"`
	Locked     *bool `yaml:"locked,omitempty" json:"locked,omitempty" doc:"Whether the account is barred from logging in. An account created without a password comes out locked."`
	// The hash is compared, never returned: see etcfiles.Shadow.HashEquals.
	PasswordHash string `yaml:"passwordHash,omitempty" json:"passwordHash,omitempty" doc:"A crypt(3) hash, never a plaintext password." docExample:"$6$rounds=..." docFlags:"writeOnly"`
}

// UserStatus is the observed state of a local account.
type UserStatus struct {
	UID          int      `yaml:"uid" json:"uid" doc:"Numeric user ID."`
	GID          int      `yaml:"gid" json:"gid" doc:"Numeric ID of the primary group."`
	PrimaryGroup string   `yaml:"primaryGroup" json:"primaryGroup" doc:"Name of the primary group, resolved from the GID."`
	Groups       []string `yaml:"groups,omitempty" json:"groups,omitempty" doc:"Supplementary groups the account belongs to."`
	Home         string   `yaml:"home" json:"home" doc:"Home directory recorded in /etc/passwd."`
	HomeExists   bool     `yaml:"homeExists" json:"homeExists" doc:"Whether that directory is actually on disk."`
	Shell        string   `yaml:"shell" json:"shell" doc:"Login shell."`
	System       bool     `yaml:"system" json:"system" doc:"Whether the UID falls outside the regular range declared in /etc/login.defs."`
	Locked       bool     `yaml:"locked" json:"locked" doc:"Whether the password field in /etc/shadow is locked. Reads as false when /etc/shadow cannot be read."`
	PasswordSet  bool     `yaml:"passwordSet" json:"passwordSet" doc:"Whether the account has a usable password."`
}

// Handler serves the User kind.
type Handler struct{ p *provider.Provider }

// New builds the handler.
func New(p *provider.Provider) core.Handler { return &Handler{p: p} }

func (h *Handler) Type() core.ResourceType {
	return provider.ResourceType(core.ResourceType{
		Kind:        "User",
		Plural:      "users",
		Singular:    "user",
		ShortNames:  []string{"usr"},
		Description: "A local login account, as recorded in /etc/passwd and /etc/shadow.",
		Columns: []core.Column{
			{Name: "NAME", Path: "metadata.name"},
			{Name: "UID", Path: "status.uid"},
			{Name: "GID", Path: "status.gid"},
			{Name: "GROUP", Path: "status.primaryGroup"},
			{Name: "SHELL", Path: "status.shell"},
			{Name: "HOME", Wide: true, Path: "status.home"},
			{Name: "GROUPS", Wide: true, Path: "status.groups"},
			{Name: "LOCKED", Wide: true, Path: "status.locked"},
			{Name: "SYSTEM", Wide: true, Path: "status.system"},
		},
	})
}

func (h *Handler) NewSpec() any { return &UserSpec{} }

func (h *Handler) NewStatus() any { return &UserStatus{} }

func (h *Handler) List(ctx context.Context) ([]core.Object, error) {
	snap, err := account.Read(h.p)
	if err != nil {
		return nil, err
	}
	out := make([]core.Object, 0, len(snap.Users))
	for _, u := range snap.Users {
		out = append(out, h.build(u, snap))
	}
	sort.Slice(out, func(i, j int) bool {
		return userStatus(out[i]).UID < userStatus(out[j]).UID
	})
	return out, nil
}

func (h *Handler) Get(ctx context.Context, name string) (core.Object, error) {
	snap, err := account.Read(h.p)
	if err != nil {
		return core.Object{}, err
	}
	u, ok := snap.User(name)
	if !ok {
		return core.Object{}, core.NotFound("user", name)
	}
	return h.build(u, snap), nil
}

func (h *Handler) Apply(ctx context.Context, obj core.Object) (core.Result, error) {
	spec, ok := obj.Spec.(*UserSpec)
	if !ok || spec == nil {
		return core.Result{}, fmt.Errorf("user %q: missing or invalid spec", obj.Metadata.Name)
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

	current, exists := snap.User(name)
	if !exists {
		if err := h.create(ctx, tools, name, spec); err != nil {
			return core.Result{}, err
		}
		obj, err := h.reload(ctx, name, obj)
		return core.Result{Action: core.ActionCreated, Object: obj, Diff: []string{"created user " + name}}, err
	}

	diff, err := h.reconcile(ctx, tools, snap, current, spec)
	if err != nil {
		return core.Result{}, err
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
	if _, found, err := account.Files(h.p).User(name); err != nil {
		return err
	} else if !found {
		return core.NotFound("user", name)
	}
	return tools.DeleteUser(ctx, name, core.DeleteOptionsFrom(ctx).Cascade)
}

// create provisions a brand new account and then applies the settings the
// creation tool cannot express (password hash, lock state).
func (h *Handler) create(ctx context.Context, tools usertools.Toolset, name string, spec *UserSpec) error {
	req := usertools.CreateUser{
		Name:         name,
		UID:          spec.UID,
		PrimaryGroup: spec.PrimaryGroup,
		Groups:       spec.Groups,
		Shell:        spec.Shell,
		Home:         spec.Home,
		Comment:      spec.Comment,
		System:       spec.System,
		CreateHome:   spec.CreateHome,
	}
	if err := tools.CreateUser(ctx, req); err != nil {
		return err
	}
	if spec.PasswordHash != "" {
		if err := tools.SetPasswordHash(ctx, name, spec.PasswordHash); err != nil {
			return err
		}
	}
	if spec.Locked != nil {
		// Freshly created accounts already come out locked (useradd leaves no
		// usable password, and `adduser -D` sets one explicitly), and both
		// toolsets fail when asked to lock an account that is already locked.
		// So compare against what creation actually produced.
		locked, known, err := account.LockState(h.p, name)
		if err != nil {
			return err
		}
		if !known || locked != *spec.Locked {
			if err := tools.SetLocked(ctx, name, *spec.Locked); err != nil {
				return err
			}
		}
	}
	return nil
}

// reconcile brings an existing account in line with the spec and reports, in
// human terms, everything it changed.
func (h *Handler) reconcile(ctx context.Context, tools usertools.Toolset, snap *account.Snapshot, current etcfiles.User, spec *UserSpec) ([]string, error) {
	var diff []string
	update := usertools.UpdateUser{}

	if spec.UID != nil && *spec.UID != current.UID {
		update.UID = spec.UID
		diff = append(diff, fmt.Sprintf("uid %d -> %d", current.UID, *spec.UID))
	}
	if spec.Shell != "" && spec.Shell != current.Shell {
		update.Shell = spec.Shell
		diff = append(diff, fmt.Sprintf("shell %s -> %s", current.Shell, spec.Shell))
	}
	if spec.Comment != "" && spec.Comment != current.Comment {
		update.Comment = spec.Comment
		diff = append(diff, fmt.Sprintf("comment %q -> %q", current.Comment, spec.Comment))
	}
	if spec.Home != "" && spec.Home != current.Home {
		update.Home = spec.Home
		update.MoveHome = true
		diff = append(diff, fmt.Sprintf("home %s -> %s (moving contents)", current.Home, spec.Home))
	}
	currentPrimary := snap.GroupName(current.GID)
	if spec.PrimaryGroup != "" && spec.PrimaryGroup != currentPrimary {
		update.PrimaryGroup = spec.PrimaryGroup
		diff = append(diff, fmt.Sprintf("primaryGroup %s -> %s", currentPrimary, spec.PrimaryGroup))
	}
	if !update.IsEmpty() {
		if err := tools.UpdateUser(ctx, current.Name, update); err != nil {
			return nil, err
		}
	}

	// A nil Groups means "not managed here"; an empty (but present) list means
	// "no supplementary groups", so it does remove memberships.
	if spec.Groups != nil {
		add, remove := account.DiffSets(snap.SupplementaryGroups(current.Name), spec.Groups)
		for _, g := range add {
			if err := tools.AddUserToGroup(ctx, current.Name, g); err != nil {
				return nil, err
			}
			diff = append(diff, "added to group "+g)
		}
		for _, g := range remove {
			if err := tools.RemoveUserFromGroup(ctx, current.Name, g); err != nil {
				return nil, err
			}
			diff = append(diff, "removed from group "+g)
		}
	}

	shadow := snap.Shadows[current.Name]
	if spec.PasswordHash != "" && !shadow.HashEquals(spec.PasswordHash) {
		if err := tools.SetPasswordHash(ctx, current.Name, spec.PasswordHash); err != nil {
			return nil, err
		}
		diff = append(diff, "password hash updated")
	}
	// The lock check comes last: setting a password unlocks the account, so
	// reasserting the desired lock state afterwards keeps apply convergent.
	if spec.Locked != nil && *spec.Locked != shadow.Locked {
		if err := tools.SetLocked(ctx, current.Name, *spec.Locked); err != nil {
			return nil, err
		}
		diff = append(diff, fmt.Sprintf("locked %t -> %t", shadow.Locked, *spec.Locked))
	}

	return diff, nil
}

// reload re-reads the object after a mutation. Under dry-run nothing actually
// changed on disk, so the object sent in is echoed back instead.
func (h *Handler) reload(ctx context.Context, name string, sent core.Object) (core.Object, error) {
	if h.p.Runner.DryRun {
		return sent, nil
	}
	return h.Get(ctx, name)
}

func (h *Handler) build(u etcfiles.User, snap *account.Snapshot) core.Object {
	primary := snap.GroupName(u.GID)
	groups := snap.SupplementaryGroups(u.Name)
	shadow := snap.Shadows[u.Name]
	system := snap.UIDRange.IsSystem(u.UID)
	locked := shadow.Locked

	uid := u.UID
	t := h.Type()
	return core.Object{
		APIVersion: t.APIVersion(),
		Kind:       t.Kind,
		Metadata:   core.Metadata{Name: u.Name},
		Spec: &UserSpec{
			UID:          &uid,
			PrimaryGroup: primary,
			Groups:       groups,
			Shell:        u.Shell,
			Home:         u.Home,
			Comment:      u.Comment,
			System:       system,
			Locked:       &locked,
		},
		Status: &UserStatus{
			UID:          u.UID,
			GID:          u.GID,
			PrimaryGroup: primary,
			Groups:       groups,
			Home:         u.Home,
			HomeExists:   dirExists(snap.Root, u.Home),
			Shell:        u.Shell,
			System:       system,
			Locked:       locked,
			PasswordSet:  shadow.PasswordSet,
		},
	}
}

func userStatus(o core.Object) *UserStatus {
	if s, ok := o.Status.(*UserStatus); ok && s != nil {
		return s
	}
	return &UserStatus{}
}

func dirExists(root, path string) bool {
	if path == "" {
		return false
	}
	if root != "" && root != "/" {
		path = root + path
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
