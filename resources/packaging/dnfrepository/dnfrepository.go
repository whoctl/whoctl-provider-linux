// Package dnfrepository manages the repositories dnf reads from.
//
// The verbs are in internal/repokind, shared with the other three repository
// kinds. What is here is what differs: the spec this manager can express, and
// the translation between it and the file on disk.
package dnfrepository

import (
	"fmt"

	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/pkgtools"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/repokind"
)

// DnfRepositorySpec is the desired state of a section in a .repo file.
type DnfRepositorySpec struct {
	Enabled     *bool    `yaml:"enabled,omitempty" json:"enabled,omitempty" doc:"Whether dnf should use this repository. Defaults to true, matching dnf's own reading of a missing enabled key."`
	DisplayName string   `yaml:"displayName,omitempty" json:"displayName,omitempty" doc:"The human-readable name, written as the repository's name= key." docExample:"Docker CE Stable"`
	BaseURL     []string `yaml:"baseurl,omitempty" json:"baseurl,omitempty" doc:"One or more archive URLs. Give this, a metalink or a mirrorlist." docExample:"https://download.docker.com/linux/fedora/$releasever/$basearch/stable"`
	Metalink    string   `yaml:"metalink,omitempty" json:"metalink,omitempty" doc:"A metalink URL, as Fedora's own repositories use."`
	Mirrorlist  string   `yaml:"mirrorlist,omitempty" json:"mirrorlist,omitempty" doc:"A mirrorlist URL."`
	GPGCheck    *bool    `yaml:"gpgcheck,omitempty" json:"gpgcheck,omitempty" doc:"Whether package signatures are verified. Left alone when omitted."`
	GPGKey      string   `yaml:"gpgkey,omitempty" json:"gpgkey,omitempty" doc:"Where the signing key lives." docExample:"https://download.docker.com/linux/fedora/gpg"`
	File        string   `yaml:"file,omitempty" json:"file,omitempty" doc:"Which .repo file to write the section into. Defaults to <name>.repo and is only read when the repository is created." docFlags:"createOnly" docExample:"docker-ce.repo"`
}

// DnfRepositoryStatus is the observed state of the same section.
type DnfRepositoryStatus struct {
	Enabled     bool     `yaml:"enabled" json:"enabled" doc:"Whether dnf uses this repository."`
	DisplayName string   `yaml:"displayName,omitempty" json:"displayName,omitempty" doc:"The repository's name= key."`
	BaseURL     []string `yaml:"baseurl,omitempty" json:"baseurl,omitempty" doc:"The archive URLs."`
	Metalink    string   `yaml:"metalink,omitempty" json:"metalink,omitempty" doc:"The metalink URL, when the repository uses one."`
	Mirrorlist  string   `yaml:"mirrorlist,omitempty" json:"mirrorlist,omitempty" doc:"The mirrorlist URL, when the repository uses one."`
	GPGCheck    bool     `yaml:"gpgcheck" json:"gpgcheck" doc:"Whether package signatures are verified."`
	GPGKey      string   `yaml:"gpgkey,omitempty" json:"gpgkey,omitempty" doc:"Where the signing key lives."`
	File        string   `yaml:"file" json:"file" doc:"The .repo file holding the section." docExample:"/etc/yum.repos.d/fedora.repo"`
}

// New builds the handler.
func New(p *provider.Provider) core.Handler {
	cfg := repokind.Config{Manager: "dnf"}
	cfg.Kind, cfg.Plural, cfg.Singular = "DnfRepository", "dnfrepositories", "dnfrepository"
	cfg.Short = []string{"dnfrepo", "yumrepo"}
	cfg.Summary = "A repository section in /etc/yum.repos.d, named by its id."
	cfg.Columns = []core.Column{
		{Name: "NAME", Path: "metadata.name"},
		{Name: "ENABLED", Path: "status.enabled"},
		{Name: "GPGCHECK", Path: "status.gpgcheck"},
		{Name: "DISPLAYNAME", Path: "status.displayName"},
		// A dnf repository points at exactly one of these three, in this order of
		// preference, which is also the order dnf itself reads them in.
		{Name: "URL", Wide: true, Path: "status.baseurl|status.metalink|status.mirrorlist", Format: core.FormatFirst},
		{Name: "FILE", Wide: true, Path: "status.file"},
	}
	cfg.Spec = func() any { return &DnfRepositorySpec{} }
	cfg.Status = func() any { return &DnfRepositoryStatus{} }
	cfg.ToRepo = func(name string, spec any) (pkgtools.Repository, error) {
		s, ok := spec.(*DnfRepositorySpec)
		if !ok || s == nil {
			return pkgtools.Repository{}, fmt.Errorf("missing or invalid spec")
		}
		if len(s.BaseURL) == 0 && s.Metalink == "" && s.Mirrorlist == "" {
			return pkgtools.Repository{}, fmt.Errorf("needs a baseurl, a metalink or a mirrorlist")
		}
		return pkgtools.Repository{
			Name:        name,
			URLs:        s.BaseURL,
			DisplayName: s.DisplayName,
			Metalink:    s.Metalink,
			Mirrorlist:  s.Mirrorlist,
			GPGCheck:    s.GPGCheck,
			GPGKey:      s.GPGKey,
			File:        s.File,
			Enabled:     repokind.BoolOr(s.Enabled, true),
		}, nil
	}
	cfg.FromRepo = func(r pkgtools.Repository) (any, any) {
		gpgCheck := false
		if r.GPGCheck != nil {
			gpgCheck = *r.GPGCheck
		}
		spec := &DnfRepositorySpec{
			Enabled: repokind.BoolPtr(r.Enabled), DisplayName: r.DisplayName, BaseURL: r.URLs,
			Metalink: r.Metalink, Mirrorlist: r.Mirrorlist, GPGCheck: r.GPGCheck, GPGKey: r.GPGKey,
		}
		return spec, &DnfRepositoryStatus{
			Enabled: r.Enabled, DisplayName: r.DisplayName, BaseURL: r.URLs,
			Metalink: r.Metalink, Mirrorlist: r.Mirrorlist, GPGCheck: gpgCheck, GPGKey: r.GPGKey,
			File: r.File,
		}
	}
	return repokind.New(p, cfg)
}
