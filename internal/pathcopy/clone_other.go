//go:build !darwin

package pathcopy

import "errors"

// cloneTree is unsupported off macOS; Copy falls through to rsync/cp.
func cloneTree(src, dst string) error {
	return errors.New("clonefile not supported on this platform")
}
