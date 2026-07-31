// Command whoctl-provider-linux serves the linux provider over whoctl's
// provider protocol, on stdin and stdout.
//
// This is what a provider binary is, and it is deliberately this short: the
// provider itself knows nothing about the protocol, and the protocol knows
// nothing about accounts.
package main

import (
	"fmt"
	"os"

	"github.com/whoctl/whoctl-provider-linux/internal/linux"
	"github.com/whoctl/whoctl-sdk-go/core"
	"github.com/whoctl/whoctl-sdk-go/docs"
	"github.com/whoctl/whoctl-sdk-go/protocol"
	"github.com/whoctl/whoctl-sdk-go/sysexec"
)

// version is stamped at build time with -ldflags.
var version = "dev"

func main() {
	// A release publishes this provider's documentation alongside its binary,
	// and the binary is what has it: the pages are embedded here and the field
	// tables come from this package's own structs, and the site fetches the
	// result.
	if len(os.Args) > 1 && os.Args[1] == "--docs-bundle" {
		if err := docs.WriteBundle(os.Stdout, linux.New(linux.Options{}), version); err != nil {
			fmt.Fprintln(os.Stderr, "whoctl-provider-linux:", err)
			os.Exit(1)
		}
		return
	}

	err := protocol.ServeProcess(func(cfg protocol.Config) (core.Provider, error) {
		// The runner is built from the session's configuration, which is what
		// makes --dry-run and -v mean something on this side of the pipe. Its
		// output goes to stderr, which whoctl passes straight through.
		runner := &sysexec.Runner{DryRun: cfg.DryRun, Verbose: cfg.Verbose, Out: os.Stderr}
		return linux.New(linux.Options{Root: cfg.Root, Runner: runner}), nil
	}, version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "whoctl-provider-linux:", err)
		os.Exit(1)
	}
}
