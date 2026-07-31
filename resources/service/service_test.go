package service

import (
	"context"
	"github.com/whoctl/whoctl-provider-linux/internal/linuxtest"
	"github.com/whoctl/whoctl-sdk-go/core"
	"reflect"
	"testing"
)

// The fixture root has no /run/systemd/system, so Detect picks OpenRC and the
// whole service read path is exercised against files instead of a live init.
func TestServiceListReadsOpenRCFromTheFilesystem(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	objs, err := h.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	byName := map[string]core.Object{}
	for _, o := range objs {
		byName[o.Metadata.Name] = o
	}
	// functions.sh is a shell library, not a service, and must not show up.
	if _, ok := byName["functions.sh"]; ok {
		t.Error("functions.sh was listed as a service")
	}
	for _, want := range []string{"crond", "sshd", "syslog", "nodesc"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("service %q missing from the listing", want)
		}
	}

	crond := serviceStatus(byName["crond"])
	if !crond.Enabled {
		t.Error("crond is symlinked into a runlevel, so it is enabled")
	}
	if !reflect.DeepEqual(crond.Runlevels, []string{"boot", "default"}) {
		t.Errorf("crond runlevels = %v, want [boot default]", crond.Runlevels)
	}
	if crond.State != StateStopped {
		t.Errorf("crond state = %q, want stopped (no marker in /run/openrc/started)", crond.State)
	}
	if crond.Description != "Periodic command scheduler" {
		t.Errorf("crond description = %q", crond.Description)
	}
	if crond.InitSystem != "openrc" {
		t.Errorf("initSystem = %q, want openrc", crond.InitSystem)
	}

	syslog := serviceStatus(byName["syslog"])
	if syslog.State != StateRunning {
		t.Error("syslog has a marker in /run/openrc/started, so it is running")
	}
	// Single-quoted descriptions are just as valid in an init script.
	if syslog.Description != "System logger" {
		t.Errorf("syslog description = %q", syslog.Description)
	}

	sshd := serviceStatus(byName["sshd"])
	if sshd.Enabled {
		t.Error("sshd is in no runlevel, so it is disabled")
	}
	if got := serviceStatus(byName["nodesc"]).Description; got != "" {
		t.Errorf("description = %q, want empty for a script without one", got)
	}
}

func TestServiceGetUnknownReturnsNotFound(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	_, err := h.Get(context.Background(), "nosuchservice")
	if !core.IsNotFound(err) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
}

func TestServiceSpecRoundTripsTheObservedState(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	obj, err := h.Get(context.Background(), "crond")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	spec, ok := obj.Spec.(*ServiceSpec)
	if !ok {
		t.Fatalf("spec has type %T, want *ServiceSpec", obj.Spec)
	}
	if spec.Enabled == nil || !*spec.Enabled {
		t.Errorf("spec.enabled = %v, want true", spec.Enabled)
	}
	if spec.State != StateStopped {
		t.Errorf("spec.state = %q, want stopped", spec.State)
	}
}

func TestServiceDeleteIsRefusedWithAnExplanation(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	err := h.Delete(context.Background(), "crond")
	if err == nil {
		t.Fatal("expected delete to be refused")
	}
	if core.IsNotFound(err) {
		t.Fatalf("err = %v, want the refusal, not not-found", err)
	}
}

func TestNormalizeState(t *testing.T) {
	for input, want := range map[string]string{
		"":        "",
		"running": StateRunning,
		"started": StateRunning,
		"Active":  StateRunning,
		"stopped": StateStopped,
		"STOPPED": StateStopped,
	} {
		got, err := normalizeState(input)
		if err != nil {
			t.Errorf("normalizeState(%q): %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeState(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := normalizeState("enabled"); err == nil {
		t.Error("normalizeState(\"enabled\") should fail: it is not a state")
	}
}
