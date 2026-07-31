// Package apkpackage manages packages under apk.
//
// The kind is its own even though the handler is shared: a manifest saying
// ApkPackage cannot be mistaken for one that would work on another distribution,
// which is the whole reason there is no single Package kind.
//
// The verbs are in internal/pkgkind, shared with the other three package kinds:
// installing under apk and under apt differ in the backend, not in what apply
// has to decide. Copying them here would be four places for the apk apply to
// drift from the apt one without anybody noticing.
package apkpackage

import (
	"github.com/whoctl/whoctl-sdk-go/core"

	"github.com/whoctl/whoctl-provider-linux/internal/provider"
	"github.com/whoctl/whoctl-provider-linux/resources/packaging/pkgkind"
)

// New builds the handler.
func New(p *provider.Provider) core.Handler {
	return pkgkind.New(p, "ApkPackage", "apkpackages", "apk",
		"A package managed by Alpine's apk.",
		"apk")
}
