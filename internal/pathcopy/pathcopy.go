// Package pathcopy copies gitignored paths (e.g. node_modules) into a new
// worktree, preferring an instant APFS copy-on-write clone of the whole tree and
// falling back to rsync or cp.
package pathcopy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Copy clones src into dst. dst's parent directories are created as needed.
//
// It returns the method used ("clone", "rsync" or "copy") and whether anything
// was copied. If src does not exist, or dst already exists, it is a no-op and
// copied is false.
func Copy(src, dst string) (method string, copied bool, err error) {
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return "", false, nil // nothing to copy; skip silently
		}
		return "", false, err
	}
	if _, err := os.Stat(dst); err == nil {
		return "", false, nil // already present in the worktree
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", false, err
	}

	for _, s := range strategies() {
		if s.fn(src, dst) == nil {
			return s.name, true, nil
		}
		// Clean up a partial copy before trying the next strategy.
		_ = os.RemoveAll(dst)
	}
	return "", false, fmt.Errorf("failed to copy %s -> %s", src, dst)
}

type strategy struct {
	name string
	fn   func(src, dst string) error
}

// strategies returns copy methods in preference order. cloneTree is provided per
// platform (an instant CoW clone on macOS/APFS, unsupported elsewhere).
func strategies() []strategy {
	return []strategy{
		{"clone", cloneTree},
		{"rsync", rsync},
		{"copy", cpRecursive},
	}
}

func rsync(src, dst string) error {
	if _, err := exec.LookPath("rsync"); err != nil {
		return err
	}
	// Trailing slash on src copies its contents into dst.
	return exec.Command("rsync", "-a", src+string(os.PathSeparator), dst+string(os.PathSeparator)).Run()
}

func cpRecursive(src, dst string) error {
	return exec.Command("cp", "-R", src, dst).Run()
}
