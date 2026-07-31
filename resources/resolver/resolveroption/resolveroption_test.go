package resolveroption

import (
	"context"
	"github.com/whoctl/whoctl-provider-linux/internal/linuxtest"
	"github.com/whoctl/whoctl-sdk-go/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolverOptionListSplitsNameFromValue(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	if got := strings.Join(linuxtest.Names(t, h), ","); got != "ndots,timeout" {
		t.Errorf("options = %s, want ndots,timeout", got)
	}

	obj, err := h.Get(context.Background(), "ndots")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	status := resolverOptionStatus(obj)
	if status.Value != "2" || status.Flag {
		t.Errorf("status = %+v, want value 2 and not a flag", status)
	}
}

func TestResolverOptionApplyUpdatesInPlaceAndIsIdempotent(t *testing.T) {
	p := linuxtest.Writable(t, "etc/resolv.conf")
	h := New(p)
	ctx := context.Background()

	obj := core.Object{Metadata: core.Metadata{Name: "ndots"}, Spec: &ResolverOptionSpec{Value: "5"}}
	result, err := h.Apply(ctx, obj)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Action != core.ActionConfigured {
		t.Errorf("action = %q, want configured", result.Action)
	}
	if result, err = h.Apply(ctx, obj); err != nil || result.Action != core.ActionUnchanged {
		t.Errorf("second apply: action = %q, err = %v", result.Action, err)
	}

	// Changing a value must not add a second entry for the same option.
	if got := strings.Join(linuxtest.Names(t, h), ","); got != "ndots,timeout" {
		t.Errorf("options = %s, want the same two", got)
	}
	data, _ := os.ReadFile(filepath.Join(p.Root, "etc", "resolv.conf"))
	if !strings.Contains(string(data), "options ndots:5 timeout:1") {
		t.Errorf("options line not rewritten as expected:\n%s", data)
	}
}

func TestResolverOptionSupportsValuelessFlags(t *testing.T) {
	p := linuxtest.Writable(t, "etc/resolv.conf")
	h := New(p)
	ctx := context.Background()

	obj := core.Object{Metadata: core.Metadata{Name: "rotate"}, Spec: &ResolverOptionSpec{}}
	if _, err := h.Apply(ctx, obj); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got, err := h.Get(ctx, "rotate")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if status := resolverOptionStatus(got); !status.Flag || status.Value != "" {
		t.Errorf("status = %+v, want a valueless flag", status)
	}
	data, _ := os.ReadFile(filepath.Join(p.Root, "etc", "resolv.conf"))
	if !strings.Contains(string(data), "options ndots:2 timeout:1 rotate") {
		t.Errorf("flag not appended bare:\n%s", data)
	}
}

func TestResolverOptionDeleteKeepsTheOthers(t *testing.T) {
	p := linuxtest.Writable(t, "etc/resolv.conf")
	h := New(p)
	ctx := context.Background()

	if err := h.Delete(ctx, "ndots"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := strings.Join(linuxtest.Names(t, h), ","); got != "timeout" {
		t.Errorf("options = %s, want only timeout", got)
	}
	if err := h.Delete(ctx, "ndots"); !core.IsNotFound(err) {
		t.Errorf("err = %v, want NotFoundError", err)
	}
}

func TestValidateOption(t *testing.T) {
	if err := validateOption("ndots", "2"); err != nil {
		t.Errorf("validateOption(ndots, 2) = %v", err)
	}
	if err := validateOption("rotate", ""); err != nil {
		t.Errorf("validateOption(rotate, \"\") = %v", err)
	}
	// The value belongs in spec.value, not glued into the name.
	if err := validateOption("ndots:2", ""); err == nil {
		t.Error("a name carrying its own colon should be rejected")
	}
	if err := validateOption("", "2"); err == nil {
		t.Error("an empty name should be rejected")
	}
	if err := validateOption("ndots", "2 3"); err == nil {
		t.Error("a value with a space should be rejected")
	}
}
