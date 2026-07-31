package usertools

import (
	"context"
	"strings"

	"github.com/whoctl/whoctl-sdk-go/sysexec"
)

// shadowTools drives shadow-utils: useradd, usermod, userdel, groupadd,
// groupmod, groupdel, gpasswd and chpasswd.
type shadowTools struct {
	runner *sysexec.Runner
}

func (t *shadowTools) Name() string { return "shadow-utils" }

func (t *shadowTools) CreateUser(ctx context.Context, req CreateUser) error {
	args := []string{}
	if req.UID != nil {
		args = append(args, "-u", itoa(*req.UID))
	}
	if req.PrimaryGroup != "" {
		args = append(args, "-g", req.PrimaryGroup)
	}
	if len(req.Groups) > 0 {
		args = append(args, "-G", strings.Join(req.Groups, ","))
	}
	if req.Shell != "" {
		args = append(args, "-s", req.Shell)
	}
	if req.Home != "" {
		args = append(args, "-d", req.Home)
	}
	if req.Comment != "" {
		args = append(args, "-c", req.Comment)
	}
	if req.System {
		args = append(args, "-r")
	}
	// Without -m/-M the behaviour depends on CREATE_HOME in login.defs; being
	// explicit keeps the result identical across distros.
	if req.CreateHome == nil || *req.CreateHome {
		args = append(args, "-m")
	} else {
		args = append(args, "-M")
	}
	args = append(args, req.Name)

	_, err := t.runner.Run(ctx, "useradd", args...)
	return err
}

func (t *shadowTools) UpdateUser(ctx context.Context, name string, req UpdateUser) error {
	if req.IsEmpty() {
		return nil
	}
	args := []string{}
	if req.UID != nil {
		args = append(args, "-u", itoa(*req.UID))
	}
	if req.PrimaryGroup != "" {
		args = append(args, "-g", req.PrimaryGroup)
	}
	if req.Shell != "" {
		args = append(args, "-s", req.Shell)
	}
	if req.Home != "" {
		args = append(args, "-d", req.Home)
		if req.MoveHome {
			args = append(args, "-m")
		}
	}
	if req.Comment != "" {
		args = append(args, "-c", req.Comment)
	}
	args = append(args, name)

	_, err := t.runner.Run(ctx, "usermod", args...)
	return err
}

func (t *shadowTools) DeleteUser(ctx context.Context, name string, removeHome bool) error {
	args := []string{}
	if removeHome {
		args = append(args, "-r")
	}
	args = append(args, name)
	_, err := t.runner.Run(ctx, "userdel", args...)
	return err
}

func (t *shadowTools) AddUserToGroup(ctx context.Context, user, group string) error {
	_, err := t.runner.Run(ctx, "gpasswd", "-a", user, group)
	return err
}

func (t *shadowTools) RemoveUserFromGroup(ctx context.Context, user, group string) error {
	_, err := t.runner.Run(ctx, "gpasswd", "-d", user, group)
	return err
}

// SetPasswordHash feeds the hash through chpasswd's stdin; on the command line
// it would be visible to any process via `ps`.
func (t *shadowTools) SetPasswordHash(ctx context.Context, user, hash string) error {
	_, err := t.runner.RunWithStdin(ctx, user+":"+hash+"\n", "chpasswd", "-e")
	return err
}

func (t *shadowTools) SetLocked(ctx context.Context, user string, locked bool) error {
	flag := "-U"
	if locked {
		flag = "-L"
	}
	_, err := t.runner.Run(ctx, "usermod", flag, user)
	return err
}

func (t *shadowTools) CreateGroup(ctx context.Context, req CreateGroup) error {
	args := []string{}
	if req.GID != nil {
		args = append(args, "-g", itoa(*req.GID))
	}
	if req.System {
		args = append(args, "-r")
	}
	args = append(args, req.Name)
	_, err := t.runner.Run(ctx, "groupadd", args...)
	return err
}

func (t *shadowTools) SetGroupGID(ctx context.Context, name string, gid int) error {
	_, err := t.runner.Run(ctx, "groupmod", "-g", itoa(gid), name)
	return err
}

func (t *shadowTools) DeleteGroup(ctx context.Context, name string) error {
	_, err := t.runner.Run(ctx, "groupdel", name)
	return err
}
