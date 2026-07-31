package user

import (
	"context"
	"github.com/whoctl/whoctl-provider-linux/internal/linuxtest"
	"github.com/whoctl/whoctl-sdk-go/core"
	"testing"
)

func TestUserListIsSortedByUID(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	objs, err := h.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(objs) != 6 {
		t.Fatalf("got %d users, want 6", len(objs))
	}
	want := []string{"root", "bin", "daemon", "alice", "bob", "nobody"}
	for i, name := range want {
		if objs[i].Metadata.Name != name {
			t.Errorf("position %d: got %q, want %q", i, objs[i].Metadata.Name, name)
		}
	}
}

func TestUserGetFillsSpecAndStatus(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	obj, err := h.Get(context.Background(), "alice")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	spec, ok := obj.Spec.(*UserSpec)
	if !ok {
		t.Fatalf("spec has type %T, want *UserSpec", obj.Spec)
	}
	if spec.UID == nil || *spec.UID != 1000 {
		t.Errorf("uid = %v, want 1000", spec.UID)
	}
	if spec.PrimaryGroup != "alice" {
		t.Errorf("primaryGroup = %q, want %q", spec.PrimaryGroup, "alice")
	}
	if spec.Shell != "/bin/sh" {
		t.Errorf("shell = %q, want /bin/sh", spec.Shell)
	}
	if spec.Comment != "Alice Liddell" {
		t.Errorf("comment = %q, want %q", spec.Comment, "Alice Liddell")
	}
	// The primary group is excluded, and the rest come out sorted.
	if got := spec.Groups; len(got) != 2 || got[0] != "developers" || got[1] != "wheel" {
		t.Errorf("groups = %v, want [developers wheel]", got)
	}
	// A spec must never carry credentials, or `get -o yaml` would leak them.
	if spec.PasswordHash != "" {
		t.Errorf("passwordHash = %q, want it to stay empty", spec.PasswordHash)
	}

	status := userStatus(obj)
	if status.GID != 1000 || status.System || !status.PasswordSet || status.Locked {
		t.Errorf("status = %+v, want gid 1000, regular, password set, unlocked", status)
	}
}

func TestUserLockedIsReadFromShadow(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	obj, err := h.Get(context.Background(), "bob")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	status := userStatus(obj)
	if !status.Locked {
		t.Error("bob should be locked: his hash starts with '!'")
	}
	if status.PasswordSet {
		t.Error("a locked hash must not count as a usable password")
	}
}

func TestUserSystemAccountsAreOutsideTheUIDRange(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	for name, want := range map[string]bool{
		"root":   true,
		"daemon": true,
		"nobody": true, // 65534 is above UID_MAX
		"alice":  false,
		"bob":    false,
	} {
		obj, err := h.Get(context.Background(), name)
		if err != nil {
			t.Fatalf("Get(%s): %v", name, err)
		}
		if got := userStatus(obj).System; got != want {
			t.Errorf("%s: system = %t, want %t", name, got, want)
		}
	}
}

func TestUserGetUnknownReturnsNotFound(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	_, err := h.Get(context.Background(), "nosuchuser")
	if !core.IsNotFound(err) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
}
