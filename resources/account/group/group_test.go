package group

import (
	"context"
	"github.com/whoctl/whoctl-provider-linux/internal/linuxtest"
	"github.com/whoctl/whoctl-provider-linux/resources/account"
	"github.com/whoctl/whoctl-sdk-go/core"
	"reflect"
	"testing"
)

func TestGroupGetSeparatesMembersFromPrimaryMembers(t *testing.T) {
	h := New(linuxtest.Fixture(t))

	developers, err := h.Get(context.Background(), "developers")
	if err != nil {
		t.Fatalf("Get(developers): %v", err)
	}
	status := groupStatus(developers)
	if !reflect.DeepEqual(status.Members, []string{"alice"}) {
		t.Errorf("members = %v, want [alice]", status.Members)
	}
	if len(status.PrimaryMembers) != 0 {
		t.Errorf("primaryMembers = %v, want none", status.PrimaryMembers)
	}

	// alice's own group lists nobody in /etc/group, but she belongs to it
	// through her GID.
	own, err := h.Get(context.Background(), "alice")
	if err != nil {
		t.Fatalf("Get(alice): %v", err)
	}
	status = groupStatus(own)
	if len(status.Members) != 0 {
		t.Errorf("members = %v, want none", status.Members)
	}
	if !reflect.DeepEqual(status.PrimaryMembers, []string{"alice"}) {
		t.Errorf("primaryMembers = %v, want [alice]", status.PrimaryMembers)
	}
}

func TestGroupDeleteRefusesWhenItIsAPrimaryGroup(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	// The check runs before any tooling is invoked, so nothing is executed
	// against the host here.
	err := h.Delete(context.Background(), "alice")
	if err == nil {
		t.Fatal("expected an error when deleting somebody's primary group")
	}
	if core.IsNotFound(err) {
		t.Fatalf("err = %v, want the primary-group refusal", err)
	}
}

func TestGroupGetUnknownReturnsNotFound(t *testing.T) {
	h := New(linuxtest.Fixture(t))
	_, err := h.Get(context.Background(), "nosuchgroup")
	if !core.IsNotFound(err) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
}

func TestDiffSets(t *testing.T) {
	add, remove := account.DiffSets([]string{"wheel", "docker"}, []string{"docker", "developers"})
	if !reflect.DeepEqual(add, []string{"developers"}) {
		t.Errorf("add = %v, want [developers]", add)
	}
	if !reflect.DeepEqual(remove, []string{"wheel"}) {
		t.Errorf("remove = %v, want [wheel]", remove)
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"alice", "svc-nginx", "user_1"}
	for _, name := range valid {
		if err := account.ValidateName(name); err != nil {
			t.Errorf("account.ValidateName(%q) = %v, want nil", name, err)
		}
	}
	invalid := []string{"", "-alice", "al ice", "root:x", "a/b", "with,comma"}
	for _, name := range invalid {
		if err := account.ValidateName(name); err == nil {
			t.Errorf("account.ValidateName(%q) = nil, want an error", name)
		}
	}
}
