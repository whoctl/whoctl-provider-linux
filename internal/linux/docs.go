package linux

import (
	_ "embed"

	"github.com/whoctl/whoctl-sdk-go/core"
	"github.com/whoctl/whoctl-sdk-go/docs"

	"github.com/whoctl/whoctl-provider-linux/resources/account/group"
	"github.com/whoctl/whoctl-provider-linux/resources/account/user"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/apkpackage"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/apkrepository"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/aptpackage"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/aptrepository"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/dnfpackage"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/dnfrepository"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/pacmanpackage"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/pacmanrepository"
	"github.com/whoctl/whoctl-provider-linux/resources/process"
	"github.com/whoctl/whoctl-provider-linux/resources/resolver/nameserver"
	"github.com/whoctl/whoctl-provider-linux/resources/resolver/resolveroption"
	"github.com/whoctl/whoctl-provider-linux/resources/resolver/searchdomain"
	"github.com/whoctl/whoctl-provider-linux/resources/service"
)

// The overview is the only page belonging to the provider as a whole. Every
// other page lives in the directory of the kind it documents, which is why the
// tree is assembled here: go:embed only reaches inside its own package.
//
//go:embed index.md
var indexPage string

// Docs implements core.DocumentedProvider.
func (p *Provider) Docs() core.ProviderDocs {
	return core.ProviderDocs{
		DisplayName: "Linux",
		Summary:     "The machine whoctl runs on: accounts, groups, services and resolver configuration, read from /etc and written through the native tooling.",
		Categories:  []string{"System"},
		Maturity:    "alpha",
		FS:          docs.Tree(pages()),
		// The kinds are grouped into families, so where a page lives cannot be
		// derived from the kind's singular. This is the provider saying so.
		PagePath:  pagePath,
		SourceDir: "resources",
	}
}

// pageLayout maps a kind's singular to where its page lives under resources/.
var pageLayout = map[string]string{
	"user":             "account/user/user.md",
	"group":            "account/group/group.md",
	"service":          "service/service.md",
	"process":          "process/process.md",
	"nameserver":       "resolver/nameserver/nameserver.md",
	"searchdomain":     "resolver/searchdomain/searchdomain.md",
	"resolveroption":   "resolver/resolveroption/resolveroption.md",
	"apkpackage":       "packaging/apkpackage/apkpackage.md",
	"aptpackage":       "packaging/aptpackage/aptpackage.md",
	"dnfpackage":       "packaging/dnfpackage/dnfpackage.md",
	"pacmanpackage":    "packaging/pacmanpackage/pacmanpackage.md",
	"apkrepository":    "packaging/apkrepository/apkrepository.md",
	"aptrepository":    "packaging/aptrepository/aptrepository.md",
	"dnfrepository":    "packaging/dnfrepository/dnfrepository.md",
	"pacmanrepository": "packaging/pacmanrepository/pacmanrepository.md",
}

func pagePath(singular string) string { return pageLayout[singular] }

// pages collects the documentation of every kind, keyed by where it lives.
func pages() map[string]string {
	out := map[string]string{"index.md": indexPage}
	for path, page := range map[string]string{
		"account/user/user.md":                           user.Page,
		"account/group/group.md":                         group.Page,
		"service/service.md":                             service.Page,
		"process/process.md":                             process.Page,
		"resolver/nameserver/nameserver.md":              nameserver.Page,
		"resolver/searchdomain/searchdomain.md":          searchdomain.Page,
		"resolver/resolveroption/resolveroption.md":      resolveroption.Page,
		"packaging/apkpackage/apkpackage.md":             apkpackage.Page,
		"packaging/aptpackage/aptpackage.md":             aptpackage.Page,
		"packaging/dnfpackage/dnfpackage.md":             dnfpackage.Page,
		"packaging/pacmanpackage/pacmanpackage.md":       pacmanpackage.Page,
		"packaging/apkrepository/apkrepository.md":       apkrepository.Page,
		"packaging/aptrepository/aptrepository.md":       aptrepository.Page,
		"packaging/dnfrepository/dnfrepository.md":       dnfrepository.Page,
		"packaging/pacmanrepository/pacmanrepository.md": pacmanrepository.Page,
	} {
		out[path] = page
	}
	return out
}
