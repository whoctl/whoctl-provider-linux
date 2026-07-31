// Package etcfiles reads the system account databases (/etc/passwd,
// /etc/group, /etc/shadow, /etc/login.defs).
//
// Reads go straight to the files, which is faster and more predictable than
// shelling out to getent; *writes* never happen here — the system is only ever
// mutated by the native tools (useradd, adduser and friends) the provider runs.
package etcfiles

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Default paths, relative to the root configured in Files.Root.
const (
	PasswdPath    = "etc/passwd"
	GroupPath     = "etc/group"
	ShadowPath    = "etc/shadow"
	LoginDefsPath = "etc/login.defs"
)

// Files reads the account databases under a root. An empty Root means "/", the
// running system; pointing Root somewhere else is what lets us test the parsers
// without touching the real machine.
type Files struct {
	Root string
}

func (f Files) path(rel string) string {
	root := f.Root
	if root == "" {
		root = "/"
	}
	return filepath.Join(root, rel)
}

// User is an /etc/passwd entry.
type User struct {
	Name    string
	UID     int
	GID     int
	Comment string
	Home    string
	Shell   string
}

// Group is an /etc/group entry.
type Group struct {
	Name    string
	GID     int
	Members []string
}

// Shadow is the part of /etc/shadow whoctl exposes. The password hash never
// leaves this package: only whether the account is locked or has a password.
type Shadow struct {
	Name           string
	Locked         bool
	PasswordSet    bool
	LastChangeDays int
	MinDays        int
	MaxDays        int
	WarnDays       int
	InactiveDays   int
	ExpireDays     int

	// hash stays unexported on purpose: callers can compare against it but
	// never read it, so it cannot leak into `get -o yaml` output.
	hash string
}

// HashEquals reports whether the stored password hash is exactly h. It is what
// makes `apply` with a spec.passwordHash idempotent without ever exposing the
// current hash.
func (s Shadow) HashEquals(h string) bool { return s.hash == h }

// Users reads /etc/passwd.
func (f Files) Users() ([]User, error) {
	var out []User
	err := f.eachLine(PasswdPath, func(lineNo int, fields []string) error {
		if len(fields) < 7 {
			return fmt.Errorf("line %d: expected 7 fields, got %d", lineNo, len(fields))
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil {
			return fmt.Errorf("line %d: invalid uid %q", lineNo, fields[2])
		}
		gid, err := strconv.Atoi(fields[3])
		if err != nil {
			return fmt.Errorf("line %d: invalid gid %q", lineNo, fields[3])
		}
		out = append(out, User{
			Name:    fields[0],
			UID:     uid,
			GID:     gid,
			Comment: fields[4],
			Home:    fields[5],
			Shell:   fields[6],
		})
		return nil
	})
	return out, err
}

// User returns a single /etc/passwd entry. The second return is false when the
// user does not exist.
func (f Files) User(name string) (User, bool, error) {
	users, err := f.Users()
	if err != nil {
		return User{}, false, err
	}
	for _, u := range users {
		if u.Name == name {
			return u, true, nil
		}
	}
	return User{}, false, nil
}

// Groups reads /etc/group.
func (f Files) Groups() ([]Group, error) {
	var out []Group
	err := f.eachLine(GroupPath, func(lineNo int, fields []string) error {
		if len(fields) < 4 {
			return fmt.Errorf("line %d: expected 4 fields, got %d", lineNo, len(fields))
		}
		gid, err := strconv.Atoi(fields[2])
		if err != nil {
			return fmt.Errorf("line %d: invalid gid %q", lineNo, fields[2])
		}
		out = append(out, Group{
			Name:    fields[0],
			GID:     gid,
			Members: splitMembers(fields[3]),
		})
		return nil
	})
	return out, err
}

// Group returns a single /etc/group entry.
func (f Files) Group(name string) (Group, bool, error) {
	groups, err := f.Groups()
	if err != nil {
		return Group{}, false, err
	}
	for _, g := range groups {
		if g.Name == name {
			return g, true, nil
		}
	}
	return Group{}, false, nil
}

// GroupByGID resolves a GID to its group.
func (f Files) GroupByGID(gid int) (Group, bool, error) {
	groups, err := f.Groups()
	if err != nil {
		return Group{}, false, err
	}
	for _, g := range groups {
		if g.GID == gid {
			return g, true, nil
		}
	}
	return Group{}, false, nil
}

// Shadows reads /etc/shadow. Without read permission (non-root user) it returns
// an empty map and no error: whoctl degrades to "I don't know whether it is
// locked" instead of failing.
func (f Files) Shadows() (map[string]Shadow, error) {
	out := map[string]Shadow{}
	err := f.eachLine(ShadowPath, func(lineNo int, fields []string) error {
		if len(fields) < 2 {
			return nil
		}
		hash := fields[1]
		s := Shadow{
			Name:           fields[0],
			Locked:         strings.HasPrefix(hash, "!") || strings.HasPrefix(hash, "*"),
			PasswordSet:    hash != "" && !strings.HasPrefix(hash, "!") && !strings.HasPrefix(hash, "*"),
			LastChangeDays: atoiOr(field(fields, 2), -1),
			MinDays:        atoiOr(field(fields, 3), -1),
			MaxDays:        atoiOr(field(fields, 4), -1),
			WarnDays:       atoiOr(field(fields, 5), -1),
			InactiveDays:   atoiOr(field(fields, 6), -1),
			ExpireDays:     atoiOr(field(fields, 7), -1),
			hash:           hash,
		}
		out[s.Name] = s
		return nil
	})
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrNotExist) {
		return map[string]Shadow{}, nil
	}
	return out, err
}

// UIDRange describes the UID range of regular accounts, read from login.defs.
// Accounts outside it are considered system accounts.
type UIDRange struct {
	Min int
	Max int
}

// DefaultUIDRange is used when /etc/login.defs is missing (the case on Alpine
// without shadow-utils installed).
var DefaultUIDRange = UIDRange{Min: 1000, Max: 60000}

// UIDRange reads UID_MIN/UID_MAX from /etc/login.defs, falling back to the
// default when the file is missing or does not set them.
func (f Files) UIDRange() UIDRange {
	r := DefaultUIDRange
	data, err := os.ReadFile(f.path(LoginDefsPath))
	if err != nil {
		return r
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		v, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "UID_MIN":
			r.Min = v
		case "UID_MAX":
			r.Max = v
		}
	}
	return r
}

// IsSystem reports whether a UID falls outside the regular account range.
func (r UIDRange) IsSystem(uid int) bool { return uid < r.Min || uid > r.Max }

// eachLine walks a colon-separated file, skipping blank lines and comments.
func (f Files) eachLine(rel string, fn func(lineNo int, fields []string) error) error {
	path := f.path(rel)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if err := fn(lineNo, strings.Split(line, ":")); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func splitMembers(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func field(fields []string, i int) string {
	if i < len(fields) {
		return fields[i]
	}
	return ""
}

func atoiOr(s string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fallback
	}
	return v
}
