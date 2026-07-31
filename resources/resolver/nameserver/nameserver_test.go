package nameserver

import (
	"context"
	"github.com/whoctl/whoctl-provider-linux/internal/linuxtest"
	"github.com/whoctl/whoctl-sdk-go/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNameserverListReflectsFileOrder(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	if got := strings.Join(linuxtest.Names(t, h), ","); got != "192.168.1.1,9.9.9.9" {
		t.Errorf("nameservers = %s", got)
	}

	obj, err := h.Get(context.Background(), "9.9.9.9")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	status := nameserverStatus(obj)
	if status.Priority != 2 || status.Family != "IPv4" || !status.Effective {
		t.Errorf("status = %+v, want priority 2, IPv4, effective", status)
	}
}

func TestNameserverEntriesPastMaxNSAreNotEffective(t *testing.T) {
	p := linuxtest.Writable(t, "etc/resolv.conf")
	h := New(p)
	ctx := context.Background()

	for _, addr := range []string{"1.1.1.1", "8.8.8.8"} {
		obj := core.Object{Metadata: core.Metadata{Name: addr}, Spec: &NameserverSpec{}}
		if _, err := h.Apply(ctx, obj); err != nil {
			t.Fatalf("Apply(%s): %v", addr, err)
		}
	}
	// The fixture already had two, so these land in positions 3 and 4.
	obj, err := h.Get(ctx, "8.8.8.8")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	status := nameserverStatus(obj)
	if status.Priority != 4 {
		t.Fatalf("priority = %d, want 4", status.Priority)
	}
	if status.Effective {
		t.Error("the fourth nameserver is past MAXNS and cannot be effective")
	}
}

func TestNameserverApplyIsIdempotentAndHonoursPriority(t *testing.T) {
	p := linuxtest.Writable(t, "etc/resolv.conf")
	h := New(p)
	ctx := context.Background()

	obj := core.Object{Metadata: core.Metadata{Name: "9.9.9.9"}, Spec: &NameserverSpec{}}
	result, err := h.Apply(ctx, obj)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Action != core.ActionUnchanged {
		t.Errorf("action = %q, want unchanged for an entry already present", result.Action)
	}

	first := 1
	obj.Spec = &NameserverSpec{Priority: &first}
	result, err = h.Apply(ctx, obj)
	if err != nil {
		t.Fatalf("Apply with priority: %v", err)
	}
	if result.Action != core.ActionConfigured {
		t.Errorf("action = %q, want configured after a move", result.Action)
	}
	if got := strings.Join(linuxtest.Names(t, h), ","); got != "9.9.9.9,192.168.1.1" {
		t.Errorf("order = %s, want 9.9.9.9 first", got)
	}

	// Applying the same priority again changes nothing.
	result, err = h.Apply(ctx, obj)
	if err != nil {
		t.Fatalf("Apply again: %v", err)
	}
	if result.Action != core.ActionUnchanged {
		t.Errorf("action = %q, want unchanged", result.Action)
	}
}

func TestNameserverApplyRejectsSomethingThatIsNotAnIP(t *testing.T) {
	p := linuxtest.Writable(t, "etc/resolv.conf")
	h := New(p)
	obj := core.Object{Metadata: core.Metadata{Name: "dns.example.com"}, Spec: &NameserverSpec{}}
	if _, err := h.Apply(context.Background(), obj); err == nil {
		t.Fatal("expected a hostname to be rejected")
	}
}

func TestNameserverDeleteKeepsTheRestOfTheFile(t *testing.T) {
	p := linuxtest.Writable(t, "etc/resolv.conf")
	h := New(p)
	ctx := context.Background()

	if err := h.Delete(ctx, "192.168.1.1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := strings.Join(linuxtest.Names(t, h), ","); got != "9.9.9.9" {
		t.Errorf("nameservers = %s, want only 9.9.9.9", got)
	}

	data, err := os.ReadFile(filepath.Join(p.Root, "etc", "resolv.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{"search home.arpa lab.internal", "options ndots:2 timeout:1", "# Fixture resolv.conf"} {
		if !strings.Contains(string(data), keep) {
			t.Errorf("delete dropped %q\nfile:\n%s", keep, data)
		}
	}

	if err := h.Delete(ctx, "192.168.1.1"); !core.IsNotFound(err) {
		t.Errorf("err = %v, want NotFoundError on the second delete", err)
	}
}

func TestNameserverDryRunLeavesTheFileAlone(t *testing.T) {
	p := linuxtest.Writable(t, "etc/resolv.conf")
	p.Runner.DryRun = true
	h := New(p)

	obj := core.Object{Metadata: core.Metadata{Name: "1.1.1.1"}, Spec: &NameserverSpec{}}
	result, err := h.Apply(context.Background(), obj)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Action != core.ActionCreated {
		t.Errorf("action = %q, want created (as it would be)", result.Action)
	}
	if got := strings.Join(linuxtest.Names(t, h), ","); got != "192.168.1.1,9.9.9.9" {
		t.Errorf("dry-run changed the file: %s", got)
	}
}
