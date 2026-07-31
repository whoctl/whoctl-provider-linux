// Package servicetools abstracts the init system.
//
// Same shape as usertools: one interface, one implementation per init system,
// picked at runtime. Alpine (and therefore the test container) runs OpenRC,
// while the production targets run systemd, so both have to work.
package servicetools

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/whoctl/whoctl-sdk-go/sysexec"
)

// Service is the observed state of a service, in terms both init systems share.
type Service struct {
	Name        string
	Description string
	Enabled     bool
	Running     bool
	// Runlevels lists the OpenRC runlevels the service is enabled in. Empty
	// under systemd.
	Runlevels []string
	// UnitState is the raw enablement state reported by systemd (enabled,
	// disabled, static, masked...). Empty under OpenRC.
	UnitState string
}

// Backend is the set of service operations whoctl needs.
type Backend interface {
	// Name identifies the init system ("openrc" or "systemd").
	Name() string

	List(ctx context.Context) ([]Service, error)
	Get(ctx context.Context, name string) (Service, bool, error)

	SetEnabled(ctx context.Context, svc Service, enabled bool) error
	SetRunning(ctx context.Context, name string, running bool) error
	Restart(ctx context.Context, name string) error
}

// Detect picks the init system in use under root.
//
// systemd wins when its runtime directory exists, which is the marker systemd
// itself documents for "am I running under systemd". OpenRC needs both an
// /etc/init.d and its own tooling: Debian and its derivatives keep sysvinit
// scripts in that directory without any of OpenRC's commands, and matching on
// the directory alone would hand them a backend whose every mutation fails with
// "rc-service: not found".
func Detect(runner *sysexec.Runner, root string) (Backend, error) {
	base := root
	if base == "" {
		base = "/"
	}
	if isDir(filepath.Join(base, "run/systemd/system")) {
		return &systemdBackend{runner: runner}, nil
	}
	if isDir(filepath.Join(base, "etc/init.d")) && hasOpenRC(base) {
		return &openrcBackend{runner: runner, root: base}, nil
	}
	return nil, errors.New("no supported init system found (expected systemd or OpenRC)")
}

// hasOpenRC looks for the commands the OpenRC backend actually runs. Under a
// fixture root the binaries are not there to be found, so an /etc/runlevels
// directory — which only OpenRC creates — stands in for them.
func hasOpenRC(base string) bool {
	if base != "/" {
		return isDir(filepath.Join(base, "etc/runlevels"))
	}
	return sysexec.Which("rc-service") != "" && sysexec.Which("rc-update") != ""
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
