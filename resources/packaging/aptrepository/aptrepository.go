// Package aptrepository manages the repositories apt reads from.
//
// The verbs are in internal/repokind, shared with the other three repository
// kinds. What is here is what differs: the spec this manager can express, and
// the translation between it and the file on disk.
package aptrepository

import (
	"fmt"

	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/pkgtools"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/repokind"
)

// AptRepositorySpec is the desired state of a file under sources.list.d.
type AptRepositorySpec struct {
	Enabled    *bool             `yaml:"enabled,omitempty" json:"enabled,omitempty" doc:"Whether apt should use this source. A disabled one is commented out rather than removed."`
	Type       string            `yaml:"type,omitempty" json:"type,omitempty" doc:"deb for binary packages, deb-src for sources. Defaults to deb." docExample:"deb"`
	URI        string            `yaml:"uri" json:"uri" doc:"Where the archive lives." docFlags:"required" docExample:"https://download.docker.com/linux/debian"`
	Suite      string            `yaml:"suite" json:"suite" doc:"The distribution the packages are built for." docFlags:"required" docExample:"bookworm"`
	Components []string          `yaml:"components,omitempty" json:"components,omitempty" doc:"Which parts of the archive to use." docExample:"main"`
	Options    map[string]string `yaml:"options,omitempty" json:"options,omitempty" doc:"Bracketed options on the source line, such as signed-by or arch." docExample:"signed-by: /etc/apt/keyrings/docker.asc"`
}

// AptRepositoryStatus is the observed state of the same file.
type AptRepositoryStatus struct {
	Enabled    bool              `yaml:"enabled" json:"enabled" doc:"Whether the source line is active."`
	Type       string            `yaml:"type" json:"type" doc:"deb or deb-src." docExample:"deb"`
	URI        string            `yaml:"uri" json:"uri" doc:"Where the archive lives."`
	Suite      string            `yaml:"suite" json:"suite" doc:"The distribution the packages are built for." docExample:"bookworm"`
	Components []string          `yaml:"components,omitempty" json:"components,omitempty" doc:"Which parts of the archive are in use."`
	Options    map[string]string `yaml:"options,omitempty" json:"options,omitempty" doc:"The bracketed options on the source line."`
	Format     string            `yaml:"format" json:"format" doc:"one-line for a .list file, deb822 for the stanza format, which whoctl reads but does not rewrite." docExample:"one-line"`
	File       string            `yaml:"file" json:"file" doc:"The file the entry was read from." docExample:"/etc/apt/sources.list.d/docker.list"`
}

// New builds the handler.
func New(p *provider.Provider) core.Handler {
	cfg := repokind.Config{Manager: "apt"}
	cfg.Kind, cfg.Plural, cfg.Singular = "AptRepository", "aptrepositories", "aptrepository"
	cfg.Short = []string{"aptrepo", "debrepo"}
	cfg.Summary = "An apt source under /etc/apt/sources.list.d, named by its file."
	cfg.Columns = []core.Column{
		{Name: "NAME", Path: "metadata.name"},
		{Name: "ENABLED", Path: "status.enabled"},
		{Name: "SUITE", Path: "status.suite"},
		{Name: "URI", Path: "status.uri"},
		{Name: "FORMAT", Wide: true, Path: "status.format"},
		{Name: "FILE", Wide: true, Path: "status.file"},
	}
	cfg.Spec = func() any { return &AptRepositorySpec{} }
	cfg.Status = func() any { return &AptRepositoryStatus{} }
	cfg.ToRepo = func(name string, spec any) (pkgtools.Repository, error) {
		s, ok := spec.(*AptRepositorySpec)
		if !ok || s == nil {
			return pkgtools.Repository{}, fmt.Errorf("missing or invalid spec")
		}
		sourceType := s.Type
		if sourceType == "" {
			sourceType = "deb"
		}
		if sourceType != "deb" && sourceType != "deb-src" {
			return pkgtools.Repository{}, fmt.Errorf("invalid type %q: use deb or deb-src", sourceType)
		}
		return pkgtools.Repository{
			Name:       name,
			URLs:       []string{s.URI},
			Suite:      s.Suite,
			Components: s.Components,
			Types:      []string{sourceType},
			Options:    s.Options,
			Enabled:    repokind.BoolOr(s.Enabled, true),
		}, nil
	}
	cfg.FromRepo = func(r pkgtools.Repository) (any, any) {
		format := "one-line"
		options := map[string]string{}
		for k, v := range r.Options {
			// The parser's own bookkeeping keys describe the file, not the
			// source line, and must not leak into a manifest.
			switch k {
			case "format":
				format = v
			case "extraEntries":
			default:
				options[k] = v
			}
		}
		if len(options) == 0 {
			options = nil
		}
		sourceType := "deb"
		if len(r.Types) > 0 {
			sourceType = r.Types[0]
		}
		spec := &AptRepositorySpec{
			Enabled:    repokind.BoolPtr(r.Enabled),
			Type:       sourceType,
			URI:        repokind.First(r.URLs),
			Suite:      r.Suite,
			Components: r.Components,
			Options:    options,
		}
		return spec, &AptRepositoryStatus{
			Enabled: r.Enabled, Type: sourceType, URI: repokind.First(r.URLs), Suite: r.Suite,
			Components: r.Components, Options: options, Format: format, File: r.File,
		}
	}
	return repokind.New(p, cfg)
}
