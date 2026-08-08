// Package process reads the running processes from /proc.
//
// It is the one kind here that describes something the machine is *doing*
// rather than something it is configured with, and it is read-only for a
// reason: what a process needs is to be signalled, and a signal is not a
// desired state. `apply` cannot express "send SIGHUP", and `delete` meaning
// kill would put the most destructive verb in the tool behind the least
// deliberate gesture — a keypress in a table.
package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
)

// ProcessSpec is what the process was asked to be, which for a running one is
// what it was started as.
type ProcessSpec struct {
	Command string   `yaml:"command" json:"command" doc:"The executable's own name, as the kernel reports it." docExample:"sshd"`
	Args    []string `yaml:"args,omitempty" json:"args,omitempty" doc:"The full command line, argv[0] included. A kernel thread has none."`
	User    string   `yaml:"user,omitempty" json:"user,omitempty" doc:"The user the process runs as, resolved from its real uid." docExample:"root"`
}

// ProcessStatus is what the kernel reports about it now.
type ProcessStatus struct {
	PID       int    `yaml:"pid" json:"pid" doc:"The process id, same as metadata.name."`
	PPID      int    `yaml:"ppid" json:"ppid" doc:"The parent's process id."`
	State     string `yaml:"state" json:"state" doc:"running, sleeping, waiting, zombie, stopped or idle." docExample:"sleeping"`
	Threads   int    `yaml:"threads" json:"threads" doc:"How many threads it has."`
	MemoryRSS int64  `yaml:"memoryRss" json:"memoryRss" doc:"Resident set size in bytes: what it is actually holding in memory."`
	UID       int    `yaml:"uid" json:"uid" doc:"The real user id."`
}

// Handler serves the kind.
type Handler struct{ p *provider.Provider }

// New builds the handler.
func New(p *provider.Provider) core.Handler { return &Handler{p: p} }

func (h *Handler) Type() core.ResourceType {
	return provider.ResourceType(core.ResourceType{
		Kind:       "Process",
		Plural:     "processes",
		Singular:   "process",
		ShortNames: []string{"proc"},
		Categories: []string{"runtime"},
		// Read-only, and it says so here rather than refusing after being
		// asked: there is no desired state a process can be reconciled to.
		Verbs:       []string{core.VerbGet, core.VerbList},
		Description: "A process running on this machine, read from /proc.",
		Columns: []core.Column{
			{Name: "NAME", Path: "metadata.name"},
			{Name: "COMMAND", Path: "spec.command"},
			{Name: "USER", Path: "spec.user"},
			{Name: "STATE", Path: "status.state"},
			{Name: "RSS", Path: "status.memoryRss", Format: core.FormatBytes},
			{Name: "AGE", Path: "metadata.creationTimestamp", Format: core.FormatAge},
			{Name: "PPID", Wide: true, Path: "status.ppid"},
			{Name: "THREADS", Wide: true, Path: "status.threads"},
		},
	})
}

func (h *Handler) NewSpec() any { return &ProcessSpec{} }

func (h *Handler) NewStatus() any { return &ProcessStatus{} }

// Apply and Delete are refused rather than absent, because the Handler
// interface has five verbs and a kind that cannot serve two of them still has
// to answer for them.
func (h *Handler) Apply(context.Context, core.Object) (core.Result, error) {
	return core.Result{}, core.Unsupportedf(
		"a process has no desired state to reconcile to: start it with a Service, or send it a signal with the tool that owns it")
}

// Delete would mean kill, and kill is not what delete means anywhere else in
// whoctl — everywhere else it removes configuration that can be applied again.
func (h *Handler) Delete(context.Context, string) error {
	return core.Unsupportedf(
		"whoctl does not kill processes: `delete` removes configuration that can be applied again, and a signal is neither")
}

func (h *Handler) List(ctx context.Context) ([]core.Object, error) {
	// The leading slash is what makes an empty Root mean "/": filepath.Join
	// drops empty elements, so Join("", "/proc") is /proc and Join("", "proc")
	// is a relative path into whatever directory whoctl happened to start in.
	proc := filepath.Join(h.p.Root, "/proc")
	entries, err := os.ReadDir(proc)
	if err != nil {
		return nil, core.Unavailablef("cannot read %s: %v", proc, err)
	}

	users := passwdNames(h.p.Root)
	boot := bootTime(proc)

	var out []core.Object
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			// Only the numbered directories are processes; /proc holds a great
			// deal else.
			continue
		}
		obj, err := h.read(proc, pid, users, boot)
		if err != nil {
			// A process that exited between the listing and the read is the
			// ordinary case, not a failure: /proc is a view of a moving target.
			continue
		}
		out = append(out, obj)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Status.(*ProcessStatus).PID < out[j].Status.(*ProcessStatus).PID
	})
	return out, nil
}

func (h *Handler) Get(ctx context.Context, name string) (core.Object, error) {
	pid, err := strconv.Atoi(name)
	if err != nil {
		return core.Object{}, core.Invalidf("%q is not a process id: a process is named by its pid", name)
	}
	proc := filepath.Join(h.p.Root, "/proc")
	obj, err := h.read(proc, pid, passwdNames(h.p.Root), bootTime(proc))
	if err != nil {
		return core.Object{}, core.NotFound("process", name)
	}
	return obj, nil
}

// read builds one process from the three files that describe it.
func (h *Handler) read(proc string, pid int, users map[int]string, boot time.Time) (core.Object, error) {
	dir := filepath.Join(proc, strconv.Itoa(pid))

	stat, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return core.Object{}, err
	}
	fields, err := statFields(string(stat))
	if err != nil {
		return core.Object{}, err
	}

	spec := &ProcessSpec{Command: fields.comm}
	if raw, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
		// Arguments are NUL-separated, with a trailing NUL. A kernel thread has
		// an empty cmdline, which is how it is told from a process.
		for _, arg := range strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00") {
			if arg != "" {
				spec.Args = append(spec.Args, arg)
			}
		}
	}

	status := &ProcessStatus{
		PID: pid, PPID: fields.ppid, State: stateName(fields.state),
		Threads: fields.threads, MemoryRSS: fields.rss * int64(os.Getpagesize()),
		UID: -1,
	}
	if uid, ok := ownerUID(dir); ok {
		status.UID = uid
		spec.User = users[uid]
	}

	meta := core.Metadata{Name: strconv.Itoa(pid)}
	if !boot.IsZero() {
		meta.CreationTimestamp = core.NewTime(boot.Add(fields.startTicks))
	}

	t := (&Handler{}).Type()
	return core.Object{
		APIVersion: t.APIVersion(), Kind: t.Kind,
		Metadata: meta, Spec: spec, Status: status,
	}, nil
}

// statFields is the handful of /proc/<pid>/stat this kind reads.
type parsedStat struct {
	comm       string
	state      string
	ppid       int
	threads    int
	rss        int64
	startTicks time.Duration
}

// statFields parses /proc/<pid>/stat.
//
// The second field is the executable's name in parentheses and may itself
// contain spaces and parentheses — "(a b)" is a legal command name — so it is
// cut at the *last* close parenthesis rather than split on whitespace. Getting
// this wrong shifts every field after it, which is the classic way to read this
// file incorrectly and still get plausible numbers.
func statFields(raw string) (parsedStat, error) {
	open := strings.IndexByte(raw, '(')
	close := strings.LastIndexByte(raw, ')')
	if open < 0 || close < open {
		return parsedStat{}, fmt.Errorf("malformed stat line")
	}
	out := parsedStat{comm: raw[open+1 : close]}

	// After the name, field 3 is the state; the numbering below counts from
	// there, so rest[0] is state, rest[1] is ppid, and so on.
	rest := strings.Fields(raw[close+1:])
	if len(rest) < 20 {
		return parsedStat{}, fmt.Errorf("stat line has %d fields", len(rest))
	}
	out.state = rest[0]
	out.ppid, _ = strconv.Atoi(rest[1])
	out.threads, _ = strconv.Atoi(rest[17])
	out.rss, _ = strconv.ParseInt(rest[21], 10, 64)

	// starttime is in clock ticks since boot. 100 per second is what
	// sysconf(_SC_CLK_TCK) returns on every Linux this runs on; reading it
	// properly needs cgo, and being wrong here moves an age rather than
	// breaking one.
	if ticks, err := strconv.ParseInt(rest[19], 10, 64); err == nil {
		out.startTicks = time.Duration(ticks) * time.Second / 100
	}
	return out, nil
}

// stateName spells the kernel's letter the way a person reads it.
func stateName(letter string) string {
	switch letter {
	case "R":
		return "running"
	case "S":
		return "sleeping"
	case "D":
		return "waiting"
	case "Z":
		return "zombie"
	case "T", "t":
		return "stopped"
	case "I":
		return "idle"
	case "X", "x":
		return "dead"
	}
	return letter
}

// ownerUID reads the process's real uid from the directory's owner, which is
// what the kernel sets it to.
func ownerUID(dir string) (int, bool) {
	info, err := os.Stat(dir)
	if err != nil {
		return 0, false
	}
	return uidOf(info)
}

// bootTime is when the machine started, which is what a process's start time is
// counted from.
func bootTime(proc string) time.Time {
	raw, err := os.ReadFile(filepath.Join(proc, "stat"))
	if err != nil {
		return time.Time{}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		seconds, ok := strings.CutPrefix(line, "btime ")
		if !ok {
			continue
		}
		if unix, err := strconv.ParseInt(strings.TrimSpace(seconds), 10, 64); err == nil {
			return time.Unix(unix, 0)
		}
	}
	return time.Time{}
}

// passwdNames maps uid to name, so a process can say who it runs as. It reads
// the file directly for the same reason every other kind here does: it is fast,
// predictable, and needs no tooling.
func passwdNames(root string) map[int]string {
	out := map[int]string{}
	raw, err := os.ReadFile(filepath.Join(root, "/etc/passwd"))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}
		if uid, err := strconv.Atoi(parts[2]); err == nil {
			out[uid] = parts[0]
		}
	}
	return out
}
