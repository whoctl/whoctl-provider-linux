//go:build !unix

package process

import "io/fs"

func uidOf(fs.FileInfo) (int, bool) { return 0, false }
