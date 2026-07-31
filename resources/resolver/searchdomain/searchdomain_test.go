package searchdomain

import (
	"context"
	"github.com/whoctl/whoctl-provider-linux/internal/linuxtest"
	"github.com/whoctl/whoctl-sdk-go/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchDomainListReflectsFileOrder(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	if got := strings.Join(linuxtest.Names(t, h), ","); got != "home.arpa,lab.internal" {
		t.Errorf("search domains = %s", got)
	}

	obj, err := h.Get(context.Background(), "lab.internal")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if status := searchDomainStatus(obj); status.Priority != 2 || !status.Effective {
		t.Errorf("status = %+v, want priority 2 and effective", status)
	}
}

func TestSearchDomainApplyHonoursPriorityAndIsIdempotent(t *testing.T) {
	p := linuxtest.Writable(t, "etc/resolv.conf")
	h := New(p)
	ctx := context.Background()

	first := 1
	obj := core.Object{Metadata: core.Metadata{Name: "lab.internal"}, Spec: &SearchDomainSpec{Priority: &first}}
	result, err := h.Apply(ctx, obj)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Action != core.ActionConfigured {
		t.Errorf("action = %q, want configured", result.Action)
	}
	if got := strings.Join(linuxtest.Names(t, h), ","); got != "lab.internal,home.arpa" {
		t.Errorf("order = %s, want lab.internal first", got)
	}

	result, err = h.Apply(ctx, obj)
	if err != nil {
		t.Fatalf("Apply again: %v", err)
	}
	if result.Action != core.ActionUnchanged {
		t.Errorf("action = %q, want unchanged", result.Action)
	}

	// A brand new domain with no priority is appended.
	obj = core.Object{Metadata: core.Metadata{Name: "corp.example"}, Spec: &SearchDomainSpec{}}
	if result, err = h.Apply(ctx, obj); err != nil {
		t.Fatalf("Apply new: %v", err)
	}
	if result.Action != core.ActionCreated {
		t.Errorf("action = %q, want created", result.Action)
	}
	if got := strings.Join(linuxtest.Names(t, h), ","); got != "lab.internal,home.arpa,corp.example" {
		t.Errorf("order = %s", got)
	}
}

// The whole search list lives on a single line, so a rewrite has to keep it
// that way instead of emitting one `search` line per domain.
func TestSearchDomainStaysOnOneLine(t *testing.T) {
	p := linuxtest.Writable(t, "etc/resolv.conf")
	h := New(p)

	obj := core.Object{Metadata: core.Metadata{Name: "corp.example"}, Spec: &SearchDomainSpec{}}
	if _, err := h.Apply(context.Background(), obj); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(p.Root, "etc", "resolv.conf"))
	if err != nil {
		t.Fatal(err)
	}
	var searchLines []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "search ") {
			searchLines = append(searchLines, l)
		}
	}
	if len(searchLines) != 1 {
		t.Fatalf("got %d search lines, want 1:\n%s", len(searchLines), data)
	}
	if searchLines[0] != "search home.arpa lab.internal corp.example" {
		t.Errorf("search line = %q", searchLines[0])
	}
	// And the neighbouring directives are untouched.
	if !strings.Contains(string(data), "nameserver 192.168.1.1") ||
		!strings.Contains(string(data), "options ndots:2 timeout:1") {
		t.Errorf("rewrite disturbed other directives:\n%s", data)
	}
}

func TestSearchDomainDeleteRemovesTheLineWhenLastOneGoes(t *testing.T) {
	p := linuxtest.Writable(t, "etc/resolv.conf")
	h := New(p)
	ctx := context.Background()

	for _, d := range []string{"home.arpa", "lab.internal"} {
		if err := h.Delete(ctx, d); err != nil {
			t.Fatalf("Delete(%s): %v", d, err)
		}
	}
	if got := linuxtest.Names(t, h); len(got) != 0 {
		t.Errorf("search domains = %v, want none", got)
	}

	data, err := os.ReadFile(filepath.Join(p.Root, "etc", "resolv.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "search") {
		t.Errorf("an empty search directive should be dropped entirely:\n%s", data)
	}
	if !strings.Contains(string(data), "nameserver 192.168.1.1") {
		t.Errorf("delete disturbed the nameservers:\n%s", data)
	}

	if err := h.Delete(ctx, "home.arpa"); !core.IsNotFound(err) {
		t.Errorf("err = %v, want NotFoundError", err)
	}
}

func TestValidateDomain(t *testing.T) {
	for _, ok := range []string{"home.arpa", "lab.internal", "example.com"} {
		if err := validateDomain(ok); err != nil {
			t.Errorf("validateDomain(%q) = %v, want nil", ok, err)
		}
	}
	// A space would silently turn one entry into two on the search line.
	for _, bad := range []string{"", "two domains", "-leading.dash", strings.Repeat("a", 254)} {
		if err := validateDomain(bad); err == nil {
			t.Errorf("validateDomain(%q) = nil, want an error", bad)
		}
	}
}
