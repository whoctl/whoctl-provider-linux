// Package pacmanrepository manages the repositories pacman reads from.
//
// The verbs are in internal/repokind, shared with the other three repository
// kinds. What is here is what differs: the spec this manager can express, and
// the translation between it and the file on disk.
package pacmanrepository

import (
	"fmt"

	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/pkgtools"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/repokind"
)

// PacmanRepositorySpec is the desired state of a section in /etc/pacman.conf.
type PacmanRepositorySpec struct {
	Enabled  *bool    `yaml:"enabled,omitempty" json:"enabled,omitempty" doc:"Whether pacman should use this repository. A disabled one is commented out in place, the way the shipped pacman.conf carries multilib."`
	Servers  []string `yaml:"servers,omitempty" json:"servers,omitempty" doc:"Explicit mirror URLs. Give these or an include." docExample:"https://mirror.example/archlinux/$repo/os/$arch"`
	Include  string   `yaml:"include,omitempty" json:"include,omitempty" doc:"A mirrorlist file to read servers from, which is how Arch's own repositories are configured." docExample:"/etc/pacman.d/mirrorlist"`
	SigLevel string   `yaml:"sigLevel,omitempty" json:"sigLevel,omitempty" doc:"Signature policy for this repository." docExample:"Required DatabaseOptional"`
}

// PacmanRepositoryStatus is the observed state of the same section.
type PacmanRepositoryStatus struct {
	Enabled  bool     `yaml:"enabled" json:"enabled" doc:"Whether the section is active or commented out."`
	Servers  []string `yaml:"servers,omitempty" json:"servers,omitempty" doc:"The mirror URLs listed in the section."`
	Include  string   `yaml:"include,omitempty" json:"include,omitempty" doc:"The mirrorlist the section includes."`
	SigLevel string   `yaml:"sigLevel,omitempty" json:"sigLevel,omitempty" doc:"The signature policy in force."`
	File     string   `yaml:"file" json:"file" doc:"The file the section was read from." docExample:"/etc/pacman.conf"`
}

// New builds the handler.
func New(p *provider.Provider) core.Handler {
	cfg := repokind.Config{Manager: "pacman"}
	cfg.Kind, cfg.Plural, cfg.Singular = "PacmanRepository", "pacmanrepositories", "pacmanrepository"
	cfg.Short = []string{"pacmanrepo"}
	cfg.Summary = "A repository section of /etc/pacman.conf, named by its section."
	cfg.Columns = []core.Column{
		{Name: "NAME", Path: "metadata.name"},
		{Name: "ENABLED", Path: "status.enabled"},
		{Name: "SERVERS", Path: "status.servers"},
		{Name: "INCLUDE", Path: "status.include"},
		{Name: "SIGLEVEL", Wide: true, Path: "status.sigLevel"},
	}
	cfg.Spec = func() any { return &PacmanRepositorySpec{} }
	cfg.Status = func() any { return &PacmanRepositoryStatus{} }
	cfg.ToRepo = func(name string, spec any) (pkgtools.Repository, error) {
		s, ok := spec.(*PacmanRepositorySpec)
		if !ok || s == nil {
			return pkgtools.Repository{}, fmt.Errorf("missing or invalid spec")
		}
		return pkgtools.Repository{
			Name:     name,
			URLs:     s.Servers,
			Include:  s.Include,
			SigLevel: s.SigLevel,
			Enabled:  repokind.BoolOr(s.Enabled, true),
		}, nil
	}
	cfg.FromRepo = func(r pkgtools.Repository) (any, any) {
		return &PacmanRepositorySpec{
				Enabled: repokind.BoolPtr(r.Enabled), Servers: r.URLs, Include: r.Include, SigLevel: r.SigLevel,
			}, &PacmanRepositoryStatus{
				Enabled: r.Enabled, Servers: r.URLs, Include: r.Include, SigLevel: r.SigLevel, File: r.File,
			}
	}
	return repokind.New(p, cfg)
}
