// Package resolvconf reads and rewrites /etc/resolv.conf.
//
// This is the one place in the linux provider that writes a file itself: there
// is no native tool for resolv.conf the way useradd exists for accounts. To
// keep the risk contained, everything that is not a nameserver line — search
// domains, options, comments, blank lines — is preserved verbatim, and the
// nameserver block is rewritten in place.
package resolvconf

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPath is resolv.conf's location relative to the configured root.
const DefaultPath = "etc/resolv.conf"

// Limits the C library imposes on resolv.conf. Entries past them are parsed by
// nobody, so whoctl reports which ones are effective instead of pretending the
// file is unbounded.
const (
	// MaxNameservers is MAXNS in resolv.h: glibc and musl both stop at three.
	MaxNameservers = 3
	// MaxSearchDomains is glibc's MAXDNSRCH. musl bounds the search list by
	// total length rather than by count, so this is the conservative answer.
	MaxSearchDomains = 6
)

// line is a single line of the file. Everything is kept, so a rewrite only
// touches the nameserver entries.
type line struct {
	raw       string
	directive string // "nameserver", "search", "options"...; empty for blanks and comments
	value     string
}

// Conf is a parsed resolv.conf.
type Conf struct {
	path    string
	mode    fs.FileMode
	lines   []line
	symlink string // target when resolv.conf is a symlink, empty otherwise
}

// Path returns the resolv.conf path under root.
func Path(root string) string {
	if root == "" {
		root = "/"
	}
	return filepath.Join(root, DefaultPath)
}

// Load parses resolv.conf under root.
func Load(root string) (*Conf, error) {
	path := Path(root)

	c := &Conf{path: path, mode: 0o644}
	if info, err := os.Lstat(path); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		if target, err := os.Readlink(path); err == nil {
			c.symlink = target
		}
	}
	if info, err := os.Stat(path); err == nil {
		c.mode = info.Mode().Perm()
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		c.lines = append(c.lines, parseLine(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func parseLine(raw string) line {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
		return line{raw: raw}
	}
	fields := strings.Fields(trimmed)
	return line{
		raw:       raw,
		directive: strings.ToLower(fields[0]),
		value:     strings.Join(fields[1:], " "),
	}
}

// FilePath is where this Conf was read from.
func (c *Conf) FilePath() string { return c.path }

// Nameservers lists the nameserver addresses, in file order.
func (c *Conf) Nameservers() []string {
	var out []string
	for _, l := range c.lines {
		if l.directive == "nameserver" && l.value != "" {
			out = append(out, l.value)
		}
	}
	return out
}

// Values returns the arguments of every occurrence of a directive, which is how
// `search` and `options` are read.
func (c *Conf) Values(directive string) []string {
	var out []string
	for _, l := range c.lines {
		if l.directive == directive {
			out = append(out, strings.Fields(l.value)...)
		}
	}
	return out
}

// ManagedBy names the daemon that owns this file, or "" when nothing indicates
// one. Writing to a file owned by a resolver daemon is pointless: it gets
// regenerated.
func (c *Conf) ManagedBy() string {
	if strings.Contains(c.symlink, "/run/systemd/resolve/") {
		return "systemd-resolved"
	}
	for _, l := range c.lines {
		trimmed := strings.TrimSpace(l.raw)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		lower := strings.ToLower(trimmed)
		switch {
		case strings.Contains(lower, "systemd-resolved"):
			return "systemd-resolved"
		case strings.Contains(lower, "networkmanager"):
			return "NetworkManager"
		case strings.Contains(lower, "resolvconf"):
			return "resolvconf"
		}
	}
	return ""
}

// SearchDomains lists the domains of the `search` directive, in order.
func (c *Conf) SearchDomains() []string { return c.Values("search") }

// Options lists the entries of the `options` directive, in order.
func (c *Conf) Options() []string { return c.Values("options") }

// SetNameservers replaces the nameserver entries with the given addresses.
// Every other line stays where it was.
func (c *Conf) SetNameservers(addresses []string) {
	replacement := make([]line, 0, len(addresses))
	for _, a := range addresses {
		replacement = append(replacement, line{raw: "nameserver " + a, directive: "nameserver", value: a})
	}
	c.replaceDirective("nameserver", replacement)
}

// SetSearchDomains replaces the `search` directive. Unlike nameservers, search
// is a single line carrying the whole ordered list; passing none removes the
// line entirely.
func (c *Conf) SetSearchDomains(domains []string) {
	c.replaceDirective("search", singleLine("search", domains))
}

// SetOptions replaces the `options` directive, which like search is one line
// holding every entry.
func (c *Conf) SetOptions(options []string) {
	c.replaceDirective("options", singleLine("options", options))
}

// SplitOption splits a resolver option into its name and value: "ndots:2" is
// name "ndots" and value "2", while a flag like "rotate" has no value.
func SplitOption(option string) (name, value string) {
	name, value, _ = strings.Cut(option, ":")
	return strings.TrimSpace(name), strings.TrimSpace(value)
}

// JoinOption is the inverse of SplitOption.
func JoinOption(name, value string) string {
	if value == "" {
		return name
	}
	return name + ":" + value
}

func singleLine(directive string, values []string) []line {
	if len(values) == 0 {
		return nil
	}
	joined := strings.Join(values, " ")
	return []line{{raw: directive + " " + joined, directive: directive, value: joined}}
}

// replaceDirective swaps every line of a directive for the replacement, at the
// position the first one held. When the directive was absent, the replacement
// goes at the end.
func (c *Conf) replaceDirective(directive string, replacement []line) {
	var out []line
	inserted := false
	for _, l := range c.lines {
		if l.directive != directive {
			out = append(out, l)
			continue
		}
		if !inserted {
			out = append(out, replacement...)
			inserted = true
		}
		// Further lines of the same directive are dropped: the replacement
		// stands for all of them.
	}
	if !inserted {
		out = append(out, replacement...)
	}
	c.lines = out
}

// Save writes the file back. It refuses to touch a file owned by a resolver
// daemon, since the change would be silently reverted.
func (c *Conf) Save() error {
	if owner := c.ManagedBy(); owner != "" {
		return fmt.Errorf("%s is managed by %s: changes here would be overwritten, configure %s instead",
			c.path, owner, owner)
	}

	var buf strings.Builder
	for _, l := range c.lines {
		buf.WriteString(l.raw)
		buf.WriteString("\n")
	}
	return writeFile(c.path, []byte(buf.String()), c.mode)
}

// writeFile prefers an atomic replace, falling back to rewriting in place.
//
// The fallback is not a nicety: inside a container /etc/resolv.conf is a bind
// mount, and renaming over a mount point fails with EBUSY. Same for a path
// whose directory is on another filesystem.
func writeFile(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".whoctl-resolv-*")
	if err != nil {
		// A read-only or missing directory means the in-place write will fail
		// too, but with a clearer error.
		return os.WriteFile(path, data, mode)
	}
	tmpName := tmp.Name()

	writeErr := func() error {
		defer tmp.Close()
		if _, err := tmp.Write(data); err != nil {
			return err
		}
		return tmp.Chmod(mode)
	}()
	if writeErr != nil {
		os.Remove(tmpName)
		return writeErr
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return os.WriteFile(path, data, mode)
	}
	return nil
}
