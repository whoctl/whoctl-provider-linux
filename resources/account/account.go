// Package account is what the user and group kinds share: one read of /etc per
// command, the detected toolset, and the parsers underneath.
//
// It sits at the root of the family rather than in the provider's shared state
// because nothing outside this directory reads any of it — the thirteen other
// kinds have no use for a passwd file.
package account

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
	"github.com/whoctl/whoctl-provider-linux/resources/account/etcfiles"
	"github.com/whoctl/whoctl-provider-linux/resources/account/usertools"
)

// Toolset detects the account tooling once per process.
//
// It is lazy because a read-only command must still work on a machine with
// neither useradd nor adduser in PATH, and it is cached here rather than on the
// provider because these two kinds are the only ones that ever ask.
func Toolset(p *provider.Provider) (usertools.Toolset, error) {
	toolsOnce.Do(func() {
		tools, toolsErr = usertools.Detect(p.Runner)
	})
	return tools, toolsErr
}

var (
	toolsOnce sync.Once
	tools     usertools.Toolset
	toolsErr  error
)

// LockState reports whether an account is currently locked. The second return
// is false when the answer is unknown — under dry-run, where the account was
// never created, or when /etc/shadow is unreadable.
// LockState reports whether an account is locked.
func LockState(p *provider.Provider, name string) (locked, known bool, err error) {
	if p.Runner.DryRun {
		return false, false, nil
	}
	shadows, err := Files(p).Shadows()
	if err != nil {
		return false, false, err
	}
	s, ok := shadows[name]
	return s.Locked, ok, nil
}

// Snapshot is a consistent read of the account databases. Every verb takes one
// Snapshot and answers from it, so a single `get Users` does not re-parse
// /etc/group once per user.
type Snapshot struct {
	Root     string
	Users    []etcfiles.User
	Groups   []etcfiles.Group
	Shadows  map[string]etcfiles.Shadow
	UIDRange etcfiles.UIDRange

	usersByName map[string]etcfiles.User
	groupsByGID map[int]etcfiles.Group
}

// Read takes one snapshot of /etc, so a command does not re-parse it per
// object.
func Read(p *provider.Provider) (*Snapshot, error) {
	Users, err := Files(p).Users()
	if err != nil {
		return nil, err
	}
	Groups, err := Files(p).Groups()
	if err != nil {
		return nil, err
	}
	Shadows, err := Files(p).Shadows()
	if err != nil {
		return nil, err
	}

	s := &Snapshot{
		Root:        p.Root,
		Users:       Users,
		Groups:      Groups,
		Shadows:     Shadows,
		UIDRange:    Files(p).UIDRange(),
		usersByName: make(map[string]etcfiles.User, len(Users)),
		groupsByGID: make(map[int]etcfiles.Group, len(Groups)),
	}
	for _, u := range Users {
		s.usersByName[u.Name] = u
	}
	for _, g := range Groups {
		// First entry wins when two Groups share a GID, matching the way
		// getent resolves the name of a duplicated GID.
		if _, dup := s.groupsByGID[g.GID]; !dup {
			s.groupsByGID[g.GID] = g
		}
	}
	return s, nil
}

func (s *Snapshot) User(name string) (etcfiles.User, bool) {
	u, ok := s.usersByName[name]
	return u, ok
}

func (s *Snapshot) Group(name string) (etcfiles.Group, bool) {
	for _, g := range s.Groups {
		if g.Name == name {
			return g, true
		}
	}
	return etcfiles.Group{}, false
}

// groupName resolves a GID to its group name, falling back to the numeric GID
// when no group owns it (which happens for orphaned accounts).
func (s *Snapshot) GroupName(gid int) string {
	if g, ok := s.groupsByGID[gid]; ok {
		return g.Name
	}
	return strconv.Itoa(gid)
}

// supplementaryGroups lists the Groups a user belongs to through /etc/group,
// excluding their primary group.
func (s *Snapshot) SupplementaryGroups(name string) []string {
	primaryGID := -1
	if u, ok := s.usersByName[name]; ok {
		primaryGID = u.GID
	}
	var out []string
	for _, g := range s.Groups {
		if g.GID == primaryGID {
			continue
		}
		if slices.Contains(g.Members, name) {
			out = append(out, g.Name)
		}
	}
	sort.Strings(out)
	return out
}

// primaryMembers lists the Users whose primary group is this GID. They are
// members of the group without appearing in the /etc/group member list.
func (s *Snapshot) PrimaryMembers(gid int) []string {
	var out []string
	for _, u := range s.Users {
		if u.GID == gid {
			out = append(out, u.Name)
		}
	}
	sort.Strings(out)
	return out
}

// DiffSets compares the current and desired string sets, returning what has to
// be added and what has to be removed.
func DiffSets(current, desired []string) (add, remove []string) {
	inCurrent := make(map[string]bool, len(current))
	for _, c := range current {
		inCurrent[c] = true
	}
	inDesired := make(map[string]bool, len(desired))
	for _, d := range desired {
		inDesired[d] = true
		if !inCurrent[d] {
			add = append(add, d)
		}
	}
	for _, c := range current {
		if !inDesired[c] {
			remove = append(remove, c)
		}
	}
	sort.Strings(add)
	sort.Strings(remove)
	return add, remove
}

// ValidateName rejects names the account tools would mangle or reject, with a
// clearer message than the tool's own.
func ValidateName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("metadata.name is required")
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("invalid name %q: cannot start with '-'", name)
	case strings.ContainsAny(name, ":,\t\n \\/"):
		return fmt.Errorf("invalid name %q: cannot contain spaces, ':', ',', '/' or '\\'", name)
	case len(name) > 32:
		return fmt.Errorf("invalid name %q: longer than 32 characters", name)
	}
	return nil
}

func JoinOrDash(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ",")
}

// Files is this family's view of /etc, built from the root the provider was
// given. It is a function rather than a field on the provider because only
// these two kinds read a passwd file, and the provider should not carry a
// parser for them.
func Files(p *provider.Provider) etcfiles.Files {
	return etcfiles.Files{Root: p.Root}
}
