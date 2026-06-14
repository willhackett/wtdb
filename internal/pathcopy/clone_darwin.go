//go:build darwin

package pathcopy

import "golang.org/x/sys/unix"

// cloneTree clones src to dst with a single directory-level clonefile(2) syscall.
// On APFS this clones the entire tree copy-on-write in O(1) — instant regardless
// of file count, unlike a per-file `cp -c`. dst must not already exist.
func cloneTree(src, dst string) error {
	return unix.Clonefile(src, dst, 0)
}
