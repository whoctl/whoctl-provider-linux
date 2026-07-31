package main

import (
	"testing"

	"github.com/whoctl/whoctl-sdk-go/providertest"

	"github.com/whoctl/whoctl-provider-linux/internal/linux"
)

// The whole of this provider's contract with whoctl, in one test: every column
// path names a real field, every capability it publishes is one it implements,
// and its documentation is complete and current.
//
// It reads the resource types and the embedded pages and never lists anything,
// so it is safe on the workstation. What is not safe lives in scripts/e2e.sh and
// runs in a container.
func TestConformance(t *testing.T) {
	providertest.Conformance(t, linux.New(linux.Options{}), providertest.Options{SourceRoot: "."})
}
