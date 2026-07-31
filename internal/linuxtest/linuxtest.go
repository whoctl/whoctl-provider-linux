// Package linuxtest is the fixture every resource package tests against.
//
// The fixtures are shared because the tree is: one testdata/etc that looks like
// a machine, and every kind reading its own corner of it. Copying the helper
// into fourteen packages would be fourteen places for it to drift from the
// fixture it reads.
//
// Nothing here touches the machine. Fixture is read-only against testdata/, and
// Writable copies what a test needs into t.TempDir() first — which is the rule
// this provider lives by, because its verbs change real accounts.
package linuxtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/whoctl/whoctl-sdk-go/core"
	"github.com/whoctl/whoctl-sdk-go/sysexec"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
)

// Root finds the fixture tree by walking up from the test's own directory.
//
// It searches rather than counting "../" because kinds do not all sit at the
// same depth: most are a family and then a kind, but Service is a family of
// one. A relative path that is right for user/ and wrong for service/ is a
// footgun that fails as "no supported init system found", which points nowhere
// near the actual problem.
func Root(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		candidate := filepath.Join(dir, "testdata")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no testdata directory above the test")
		}
		dir = parent
	}
}

// Fixture reads from testdata/etc instead of the real machine.
func Fixture(t *testing.T) *provider.Provider {
	t.Helper()
	return provider.New(provider.Options{Root: Root(t)})
}

// Writable copies the named files out of the fixture into a temporary root, so
// a test can apply and delete for real without touching /etc or testdata.
func Writable(t *testing.T, files ...string) *provider.Provider {
	t.Helper()
	root := t.TempDir()
	for _, name := range files {
		target := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(Root(t), name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return provider.New(provider.Options{Root: root, Runner: &sysexec.Runner{}})
}

// Names lists the names a handler returns, which is what most of these tests
// actually assert about a listing.
func Names(t *testing.T, h core.Handler) []string {
	t.Helper()
	objs, err := h.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	out := make([]string, 0, len(objs))
	for _, o := range objs {
		out = append(out, o.Metadata.Name)
	}
	return out
}
