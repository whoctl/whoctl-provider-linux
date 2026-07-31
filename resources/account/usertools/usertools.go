// Package usertools abstracts the native account management tools.
//
// Two toolsets are in use across the Linux world, with incompatible CLIs:
// shadow-utils (useradd/usermod/userdel, found on Debian, Fedora, RHEL and on
// Alpine after `apk add shadow`) and the BusyBox applets (adduser/deluser),
// which are Alpine's default. whoctl detects which one is available and
// translates operations to the right tool.
package usertools

import (
	"context"
	"errors"
	"strconv"

	"github.com/whoctl/whoctl-sdk-go/core"
	"github.com/whoctl/whoctl-sdk-go/sysexec"
)

// ErrUnsupported means the requested operation does not exist in the detected
// toolset. BusyBox, for instance, has no equivalent to usermod.
var ErrUnsupported = errors.New("operation not supported by the available toolset")

// Unsupported builds an unavailable-operation error that also says how to fix
// it.
//
// It carries core.CodeUnsupported because the answer does not depend on the
// state of the machine — BusyBox will not grow a usermod — which is exactly the
// distinction a caller needs and the one that has to survive the trip out of a
// provider's process. ErrUnsupported stays wrapped so errors.Is keeps working
// for callers inside this package.
func Unsupported(toolset, op string) error {
	return core.Unsupportedf("%w: %s does not implement %q; install shadow-utils (on Alpine: `apk add shadow`)", ErrUnsupported, toolset, op)
}

// CreateUser describes the creation of a user.
type CreateUser struct {
	Name         string
	UID          *int
	PrimaryGroup string
	Groups       []string
	Shell        string
	Home         string
	Comment      string
	System       bool
	// A nil CreateHome leaves the decision to the native tool.
	CreateHome *bool
}

// UpdateUser describes the reconciliation of an existing user. Zero fields mean
// "leave it alone".
type UpdateUser struct {
	UID          *int
	PrimaryGroup string
	Shell        string
	Home         string
	Comment      string
	// MoveHome moves the contents of the old home when Home changes.
	MoveHome bool
}

// IsEmpty reports whether there is nothing to change.
func (u UpdateUser) IsEmpty() bool {
	return u.UID == nil && u.PrimaryGroup == "" && u.Shell == "" && u.Home == "" && u.Comment == ""
}

// CreateGroup describes the creation of a group.
type CreateGroup struct {
	Name   string
	GID    *int
	System bool
}

// Toolset is the set of account mutation operations.
type Toolset interface {
	// Name identifies the toolset ("shadow-utils" or "busybox").
	Name() string

	CreateUser(ctx context.Context, req CreateUser) error
	UpdateUser(ctx context.Context, name string, req UpdateUser) error
	DeleteUser(ctx context.Context, name string, removeHome bool) error

	AddUserToGroup(ctx context.Context, user, group string) error
	RemoveUserFromGroup(ctx context.Context, user, group string) error

	SetPasswordHash(ctx context.Context, user, hash string) error
	SetLocked(ctx context.Context, user string, locked bool) error

	CreateGroup(ctx context.Context, req CreateGroup) error
	SetGroupGID(ctx context.Context, name string, gid int) error
	DeleteGroup(ctx context.Context, name string) error
}

// Detect picks the toolset available on the machine, preferring shadow-utils
// because it is more complete. It fails when neither is found.
func Detect(runner *sysexec.Runner) (Toolset, error) {
	if sysexec.Which("useradd") != "" && sysexec.Which("groupadd") != "" {
		return &shadowTools{runner: runner}, nil
	}
	if sysexec.Which("adduser") != "" && sysexec.Which("addgroup") != "" {
		return &busyboxTools{runner: runner}, nil
	}
	return nil, errors.New("no account management tooling found (expected useradd or adduser in PATH)")
}

func itoa(v int) string { return strconv.Itoa(v) }
