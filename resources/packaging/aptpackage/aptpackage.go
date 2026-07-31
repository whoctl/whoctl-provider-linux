// Package aptpackage manages packages under apt.
//
// The kind is its own even though the handler is shared: a manifest saying
// AptPackage cannot be mistaken for one that would work on another distribution,
// which is the whole reason there is no single Package kind.
//
// The verbs are in internal/pkgkind, shared with the other three package kinds:
// installing under apk and under apt differ in the backend, not in what apply
// has to decide. Copying them here would be four places for the apk apply to
// drift from the apt one without anybody noticing.
package aptpackage

import (
	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/pkgkind"
)

// New builds the handler.
func New(p *provider.Provider) core.Handler {
	return pkgkind.New(p, "AptPackage", "aptpackages", "apt",
		"A package managed by Debian's apt.",
		"apt", "deb")
}
