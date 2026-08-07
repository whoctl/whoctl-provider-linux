//go:build unix

package process

import (
	"io/fs"
	"syscall"
)

// uidOf reads a file's owner. It is per-platform because the uid is in a
// syscall-specific struct: this provider only builds for Linux, but the SDK's
// own tests build everything everywhere, and a bare type assertion on Stat_t
// would not compile where it does not exist.
func uidOf(info fs.FileInfo) (int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(stat.Uid), true
}
