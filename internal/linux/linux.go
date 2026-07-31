// Package linux assembles the provider from its resources.
//
// It is a separate package from internal/provider because the two point at each
// other otherwise: every resource needs the shared state, and the list of
// resources needs every resource. Assembly is the third place that imports
// both, and it is the only file that changes when a kind is added.
//
// The kinds are grouped into families under resources/, and a family's shared
// code sits at its root: what is inside resources/account is used by the account
// kinds and by nobody else. Two families both call their kinds apk, apt, dnf and
// pacman, which is what the import aliases are for.
package linux

import (
	"github.com/whoctl/whoctl-sdk-go/core"

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
	"github.com/whoctl/whoctl-provider-linux/resources/resolver/nameserver"
	"github.com/whoctl/whoctl-provider-linux/resources/resolver/resolveroption"
	"github.com/whoctl/whoctl-provider-linux/resources/resolver/searchdomain"
	"github.com/whoctl/whoctl-provider-linux/resources/service"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
)

// Options configures the provider.
type Options = provider.Options

// Provider is the linux provider: the shared state, plus the kinds served over
// it.
type Provider struct {
	*provider.Provider
}

// New builds it.
func New(opts Options) *Provider {
	return &Provider{Provider: provider.New(opts)}
}

// Handlers implements core.Provider, in the order `api-resources` shows them.
func (p *Provider) Handlers() []core.Handler {
	return []core.Handler{
		user.New(p.Provider),
		group.New(p.Provider),
		service.New(p.Provider),
		nameserver.New(p.Provider),
		searchdomain.New(p.Provider),
		resolveroption.New(p.Provider),
		apkpackage.New(p.Provider),
		aptpackage.New(p.Provider),
		dnfpackage.New(p.Provider),
		pacmanpackage.New(p.Provider),
		apkrepository.New(p.Provider),
		aptrepository.New(p.Provider),
		dnfrepository.New(p.Provider),
		pacmanrepository.New(p.Provider),
	}
}
