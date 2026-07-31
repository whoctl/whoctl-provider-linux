package servicetools

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/whoctl/whoctl-sdk-go/sysexec"
)

// defaultRunlevel is where `enabled: true` puts a service. OpenRC has several
// runlevels, but "default" is the one that means "start on a normal boot".
const defaultRunlevel = "default"

// openrcBackend drives OpenRC.
//
// Reads are pure filesystem lookups — /etc/init.d for what exists,
// /etc/runlevels for what is enabled, /run/openrc/started for what is running —
// so listing services costs no process spawns at all. Writes go through
// rc-update and rc-service.
type openrcBackend struct {
	runner *sysexec.Runner
	root   string
}

func (b *openrcBackend) Name() string { return "openrc" }

func (b *openrcBackend) initDir() string      { return filepath.Join(b.root, "etc/init.d") }
func (b *openrcBackend) runlevelsDir() string { return filepath.Join(b.root, "etc/runlevels") }
func (b *openrcBackend) startedDir() string   { return filepath.Join(b.root, "run/openrc/started") }

func (b *openrcBackend) List(ctx context.Context) ([]Service, error) {
	entries, err := os.ReadDir(b.initDir())
	if err != nil {
		return nil, err
	}
	runlevels, err := b.runlevels()
	if err != nil {
		return nil, err
	}
	started := b.started()

	var out []Service
	for _, e := range entries {
		name := e.Name()
		if !b.isServiceScript(name) {
			continue
		}
		out = append(out, Service{
			Name:        name,
			Description: b.description(name),
			Enabled:     len(runlevels[name]) > 0,
			Running:     started[name],
			Runlevels:   runlevels[name],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (b *openrcBackend) Get(ctx context.Context, name string) (Service, bool, error) {
	if !b.isServiceScript(name) {
		return Service{}, false, nil
	}
	runlevels, err := b.runlevels()
	if err != nil {
		return Service{}, false, err
	}
	return Service{
		Name:        name,
		Description: b.description(name),
		Enabled:     len(runlevels[name]) > 0,
		Running:     b.started()[name],
		Runlevels:   runlevels[name],
	}, true, nil
}

func (b *openrcBackend) SetEnabled(ctx context.Context, svc Service, enabled bool) error {
	if enabled {
		_, err := b.runner.Run(ctx, "rc-update", "add", svc.Name, defaultRunlevel)
		return err
	}
	// `rc-update del` without a runlevel only touches the current one, which
	// would leave the service enabled elsewhere and make apply report a change
	// forever. Remove it from every runlevel it is actually in.
	for _, level := range svc.Runlevels {
		if _, err := b.runner.Run(ctx, "rc-update", "del", svc.Name, level); err != nil {
			return err
		}
	}
	return nil
}

func (b *openrcBackend) SetRunning(ctx context.Context, name string, running bool) error {
	action := "stop"
	if running {
		action = "start"
	}
	_, err := b.runner.Run(ctx, "rc-service", name, action)
	return err
}

func (b *openrcBackend) Restart(ctx context.Context, name string) error {
	_, err := b.runner.Run(ctx, "rc-service", name, "restart")
	return err
}

// runlevels maps a service to the runlevels it is enabled in, by reading the
// symlinks under /etc/runlevels.
func (b *openrcBackend) runlevels() (map[string][]string, error) {
	out := map[string][]string{}
	levels, err := os.ReadDir(b.runlevelsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, level := range levels {
		if !level.IsDir() {
			continue
		}
		services, err := os.ReadDir(filepath.Join(b.runlevelsDir(), level.Name()))
		if err != nil {
			continue
		}
		for _, s := range services {
			out[s.Name()] = append(out[s.Name()], level.Name())
		}
	}
	for name := range out {
		sort.Strings(out[name])
	}
	return out, nil
}

// started reads OpenRC's runtime marker directory. A missing directory just
// means nothing has been started yet (or OpenRC never booted, as in a
// container), so it is not an error.
func (b *openrcBackend) started() map[string]bool {
	out := map[string]bool{}
	entries, err := os.ReadDir(b.startedDir())
	if err != nil {
		return out
	}
	for _, e := range entries {
		out[e.Name()] = true
	}
	return out
}

// isServiceScript filters out what lives in /etc/init.d without being a
// service: the shared shell libraries (functions.sh and friends) and anything
// that is not executable.
func (b *openrcBackend) isServiceScript(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".sh") {
		return false
	}
	info, err := os.Stat(filepath.Join(b.initDir(), name))
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

// descriptionRE matches the `description="..."` assignment OpenRC init scripts
// use. Reading it beats running `rc-service describe` once per service.
var descriptionRE = regexp.MustCompile(`(?m)^\s*description=(?:"([^"]*)"|'([^']*)'|(\S+))`)

func (b *openrcBackend) description(name string) string {
	data, err := os.ReadFile(filepath.Join(b.initDir(), name))
	if err != nil {
		return ""
	}
	m := descriptionRE.FindSubmatch(data)
	if m == nil {
		return ""
	}
	for _, group := range m[1:] {
		if len(group) > 0 {
			return string(group)
		}
	}
	return ""
}

// ensure the backend satisfies the interface even if the file is read alone.
var _ Backend = (*openrcBackend)(nil)
