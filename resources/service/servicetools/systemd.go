package servicetools

import (
	"context"
	"sort"
	"strings"

	"github.com/whoctl/whoctl-sdk-go/sysexec"
)

// unitSuffix is the unit type whoctl manages. Names are carried without it and
// it is added back when calling systemctl, so `whoctl get service sshd` and
// `sshd.service` are the same object.
const unitSuffix = ".service"

// systemdBackend drives systemd through systemctl.
//
// Unlike OpenRC there is no cheap filesystem view of unit state — enablement
// lives in symlink farms across several directories and activity lives in the
// manager's memory — so reads go through systemctl, in bulk for List and with
// a single `systemctl show` for Get.
type systemdBackend struct {
	runner *sysexec.Runner
}

func (b *systemdBackend) Name() string { return "systemd" }

func (b *systemdBackend) List(ctx context.Context) ([]Service, error) {
	services := map[string]*Service{}

	// list-unit-files knows every installed unit and whether it is enabled,
	// including units that were never loaded.
	out, err := b.runner.Run(ctx, "systemctl", "list-unit-files", "--type=service",
		"--no-legend", "--no-pager", "--plain")
	if err != nil {
		return nil, err
	}
	for _, fields := range splitColumns(out, 2) {
		name := trimUnit(fields[0])
		services[name] = &Service{
			Name:      name,
			UnitState: fields[1],
			Enabled:   isEnabledState(fields[1]),
		}
	}

	// list-units adds the runtime state and the description for what is loaded.
	out, err = b.runner.Run(ctx, "systemctl", "list-units", "--type=service", "--all",
		"--no-legend", "--no-pager", "--plain")
	if err != nil {
		return nil, err
	}
	for _, fields := range splitColumns(out, 4) {
		name := trimUnit(fields[0])
		svc, ok := services[name]
		if !ok {
			// A transient or generated unit: it runs without a unit file.
			svc = &Service{Name: name}
			services[name] = svc
		}
		svc.Running = fields[2] == "active"
		if len(fields) > 4 {
			svc.Description = fields[4]
		}
	}

	result := make([]Service, 0, len(services))
	for _, svc := range services {
		result = append(result, *svc)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (b *systemdBackend) Get(ctx context.Context, name string) (Service, bool, error) {
	out, err := b.runner.Run(ctx, "systemctl", "show", unitName(name), "--no-pager",
		"--property=Id",
		"--property=Description",
		"--property=UnitFileState",
		"--property=ActiveState",
		"--property=LoadState",
	)
	if err != nil {
		return Service{}, false, err
	}

	props := map[string]string{}
	for line := range strings.SplitSeq(out, "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			props[key] = value
		}
	}
	// systemd answers for unknown units too, saying the unit could not be
	// loaded; that is how "does not exist" is reported.
	if props["LoadState"] == "not-found" || props["Id"] == "" {
		return Service{}, false, nil
	}

	return Service{
		Name:        trimUnit(props["Id"]),
		Description: props["Description"],
		UnitState:   props["UnitFileState"],
		Enabled:     isEnabledState(props["UnitFileState"]),
		Running:     props["ActiveState"] == "active",
	}, true, nil
}

func (b *systemdBackend) SetEnabled(ctx context.Context, svc Service, enabled bool) error {
	action := "disable"
	if enabled {
		action = "enable"
	}
	_, err := b.runner.Run(ctx, "systemctl", action, unitName(svc.Name))
	return err
}

func (b *systemdBackend) SetRunning(ctx context.Context, name string, running bool) error {
	action := "stop"
	if running {
		action = "start"
	}
	_, err := b.runner.Run(ctx, "systemctl", action, unitName(name))
	return err
}

func (b *systemdBackend) Restart(ctx context.Context, name string) error {
	_, err := b.runner.Run(ctx, "systemctl", "restart", unitName(name))
	return err
}

// isEnabledState maps systemd's enablement vocabulary onto a boolean. "static"
// and "indirect" units have no enable/disable switch of their own, so they do
// not count as enabled; UnitState keeps the exact word for the status output.
func isEnabledState(state string) bool {
	return strings.HasPrefix(state, "enabled")
}

func unitName(name string) string {
	if strings.Contains(name, ".") {
		return name
	}
	return name + unitSuffix
}

func trimUnit(name string) string {
	return strings.TrimSuffix(name, unitSuffix)
}

// splitColumns turns systemctl's whitespace-aligned output into rows, keeping
// the trailing description column intact. Rows with fewer than min fields are
// dropped, which also gets rid of the summary lines.
func splitColumns(out string, minFields int) [][]string {
	var rows [][]string
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < minFields {
			continue
		}
		// The description is the last column and contains spaces; rejoin it.
		if len(fields) > minFields+1 && minFields == 4 {
			fields = append(fields[:4], strings.Join(fields[4:], " "))
		}
		rows = append(rows, fields)
	}
	return rows
}

var _ Backend = (*systemdBackend)(nil)
