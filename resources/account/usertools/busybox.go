package usertools

import (
	"context"

	"github.com/whoctl/whoctl-sdk-go/sysexec"
)

// busyboxTools drives the BusyBox applets, Alpine's default when the shadow
// package is not installed.
//
// BusyBox covers creation, removal and group membership, but has no equivalent
// to usermod or groupmod: changing attributes of an existing account returns
// ErrUnsupported, telling the user to install shadow-utils.
type busyboxTools struct {
	runner *sysexec.Runner
}

func (t *busyboxTools) Name() string { return "busybox" }

func (t *busyboxTools) CreateUser(ctx context.Context, req CreateUser) error {
	// -D avoids the interactive password prompt: the account starts with no
	// usable password and gets its hash afterwards, if the manifest declares
	// one.
	args := []string{"-D"}
	if req.UID != nil {
		args = append(args, "-u", itoa(*req.UID))
	}
	if req.PrimaryGroup != "" {
		args = append(args, "-G", req.PrimaryGroup)
	}
	if req.Shell != "" {
		args = append(args, "-s", req.Shell)
	}
	if req.Home != "" {
		args = append(args, "-h", req.Home)
	}
	if req.Comment != "" {
		args = append(args, "-g", req.Comment)
	}
	if req.System {
		args = append(args, "-S")
	}
	if req.CreateHome != nil && !*req.CreateHome {
		args = append(args, "-H")
	}
	args = append(args, req.Name)

	if _, err := t.runner.Run(ctx, "adduser", args...); err != nil {
		return err
	}
	// adduser only takes the primary group; the rest are added one by one.
	for _, g := range req.Groups {
		if err := t.AddUserToGroup(ctx, req.Name, g); err != nil {
			return err
		}
	}
	return nil
}

func (t *busyboxTools) UpdateUser(ctx context.Context, name string, req UpdateUser) error {
	if req.IsEmpty() {
		return nil
	}
	return Unsupported(t.Name(), "usermod")
}

func (t *busyboxTools) DeleteUser(ctx context.Context, name string, removeHome bool) error {
	args := []string{}
	if removeHome {
		args = append(args, "--remove-home")
	}
	args = append(args, name)
	_, err := t.runner.Run(ctx, "deluser", args...)
	return err
}

func (t *busyboxTools) AddUserToGroup(ctx context.Context, user, group string) error {
	_, err := t.runner.Run(ctx, "addgroup", user, group)
	return err
}

func (t *busyboxTools) RemoveUserFromGroup(ctx context.Context, user, group string) error {
	_, err := t.runner.Run(ctx, "delgroup", user, group)
	return err
}

func (t *busyboxTools) SetPasswordHash(ctx context.Context, user, hash string) error {
	_, err := t.runner.RunWithStdin(ctx, user+":"+hash+"\n", "chpasswd", "-e")
	return err
}

func (t *busyboxTools) SetLocked(ctx context.Context, user string, locked bool) error {
	flag := "-u"
	if locked {
		flag = "-l"
	}
	_, err := t.runner.Run(ctx, "passwd", flag, user)
	return err
}

func (t *busyboxTools) CreateGroup(ctx context.Context, req CreateGroup) error {
	args := []string{}
	if req.GID != nil {
		args = append(args, "-g", itoa(*req.GID))
	}
	if req.System {
		args = append(args, "-S")
	}
	args = append(args, req.Name)
	_, err := t.runner.Run(ctx, "addgroup", args...)
	return err
}

func (t *busyboxTools) SetGroupGID(ctx context.Context, name string, gid int) error {
	return Unsupported(t.Name(), "groupmod")
}

func (t *busyboxTools) DeleteGroup(ctx context.Context, name string) error {
	_, err := t.runner.Run(ctx, "delgroup", name)
	return err
}
