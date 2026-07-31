package searchdomain

import _ "embed"

// Page is this kind's documentation, embedded so it travels with the binary and
// reaches the site as part of the provider's bundle.
//
// It lives here rather than in a documentation tree of its own because
// everything about a kind belongs in one directory: the spec, the handler, the
// tests, the example and the prose.
//
//go:embed searchdomain.md
var Page string
