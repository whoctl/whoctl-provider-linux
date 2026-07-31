// Package apkrepository manages the repositories apk reads from.
//
// The verbs are in internal/repokind, shared with the other three repository
// kinds. What is here is what differs: the spec this manager can express, and
// the translation between it and the file on disk.
package apkrepository

import (
	"fmt"

	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/pkgtools"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/repokind"
)

// ApkRepositorySpec is the desired state of a line in /etc/apk/repositories.
type ApkRepositorySpec struct {
	Enabled *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty" doc:"Whether apk should use this repository. A disabled one is kept in the file, commented out, so the URL is not lost."`
	Tag     string `yaml:"tag,omitempty" json:"tag,omitempty" doc:"Optional tag, which makes the repository addressable as pkg@tag when installing." docExample:"edge"`
}

// ApkRepositoryStatus is the observed state of the same line.
type ApkRepositoryStatus struct {
	URL     string `yaml:"url" json:"url" doc:"The repository URL, same as metadata.name." docExample:"https://dl-cdn.alpinelinux.org/alpine/v3.22/main"`
	Enabled bool   `yaml:"enabled" json:"enabled" doc:"Whether the line is active or commented out."`
	Tag     string `yaml:"tag,omitempty" json:"tag,omitempty" doc:"The tag the repository is addressable by." docExample:"edge"`
	File    string `yaml:"file" json:"file" doc:"The file the entry was read from." docExample:"/etc/apk/repositories"`
}

// New builds the handler.
func New(p *provider.Provider) core.Handler {
	cfg := repokind.Config{Manager: "apk"}
	cfg.Kind, cfg.Plural, cfg.Singular = "ApkRepository", "apkrepositories", "apkrepository"
	cfg.Short = []string{"apkrepo"}
	cfg.Summary = "A package source in /etc/apk/repositories, named by its URL."
	cfg.Columns = []core.Column{
		{Name: "NAME", Path: "metadata.name"},
		{Name: "ENABLED", Path: "status.enabled"},
		{Name: "TAG", Path: "status.tag"},
		{Name: "FILE", Wide: true, Path: "status.file"},
	}
	cfg.Spec = func() any { return &ApkRepositorySpec{} }
	cfg.Status = func() any { return &ApkRepositoryStatus{} }
	cfg.ToRepo = func(name string, spec any) (pkgtools.Repository, error) {
		s, ok := spec.(*ApkRepositorySpec)
		if !ok || s == nil {
			return pkgtools.Repository{}, fmt.Errorf("missing or invalid spec")
		}
		return pkgtools.Repository{
			Name:    name,
			URLs:    []string{name},
			Enabled: repokind.BoolOr(s.Enabled, true),
			Tag:     s.Tag,
		}, nil
	}
	cfg.FromRepo = func(r pkgtools.Repository) (any, any) {
		return &ApkRepositorySpec{Enabled: repokind.BoolPtr(r.Enabled), Tag: r.Tag},
			&ApkRepositoryStatus{URL: r.Name, Enabled: r.Enabled, Tag: r.Tag, File: r.File}
	}
	return repokind.New(p, cfg)
}
