// Package git wraps the handful of git operations wtdb needs: locating the main
// worktree, listing linked worktrees, and installing the post-checkout hook.
package git

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotRepo is returned when a git command runs outside a git repository.
var ErrNotRepo = errors.New("not inside a git repository")

// Worktree is one entry from `git worktree list`.
type Worktree struct {
	Path   string // absolute path to the worktree
	Branch string // short branch name, or "" if detached
	Head   string // commit SHA
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		stderr := strings.TrimSpace(errb.String())
		if strings.Contains(stderr, "not a git repository") {
			return "", ErrNotRepo
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr)
	}
	return strings.TrimSpace(out.String()), nil
}

// Toplevel returns the absolute path of the current worktree's root.
func Toplevel(dir string) (string, error) {
	return run(dir, "rev-parse", "--show-toplevel")
}

// CommonDir returns the shared .git directory (the same for all worktrees of a repo).
func CommonDir(dir string) (string, error) {
	p, err := run(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return p, nil
}

// MainWorktree returns the path of the primary (non-linked) worktree, which is
// the parent of the common .git directory.
func MainWorktree(dir string) (string, error) {
	common, err := CommonDir(dir)
	if err != nil {
		return "", err
	}
	return filepath.Dir(common), nil
}

// IsLinkedWorktree reports whether dir is a linked worktree (created via
// `git worktree add`) rather than the main worktree.
func IsLinkedWorktree(dir string) (bool, error) {
	main, err := MainWorktree(dir)
	if err != nil {
		return false, err
	}
	top, err := Toplevel(dir)
	if err != nil {
		return false, err
	}
	return top != main, nil
}

// CurrentBranch returns the short branch name of the worktree at dir.
func CurrentBranch(dir string) (string, error) {
	return run(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// ListWorktrees parses `git worktree list --porcelain`.
func ListWorktrees(dir string) ([]Worktree, error) {
	out, err := run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var wts []Worktree
	var cur *Worktree
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "worktree "):
			if cur != nil {
				wts = append(wts, *cur)
			}
			cur = &Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "HEAD "):
			if cur != nil {
				cur.Head = strings.TrimPrefix(line, "HEAD ")
			}
		case strings.HasPrefix(line, "branch "):
			if cur != nil {
				ref := strings.TrimPrefix(line, "branch ")
				cur.Branch = strings.TrimPrefix(ref, "refs/heads/")
			}
		}
	}
	if cur != nil {
		wts = append(wts, *cur)
	}
	return wts, sc.Err()
}

const hookMarkerBegin = "# >>> wtdb hook >>>"
const hookMarkerEnd = "# <<< wtdb hook <<<"

// hookBlock is the marked block wtdb manages inside post-checkout.
func hookBlock() string {
	return strings.Join([]string{
		hookMarkerBegin,
		"# Managed by wtdb. Runs the create flow when a worktree is added.",
		`command -v wtdb >/dev/null 2>&1 && wtdb hook post-checkout "$@"`,
		hookMarkerEnd,
	}, "\n")
}

// InstallHook installs (or refreshes) the post-checkout hook in the repo's shared
// hooks directory. It preserves any existing hook content by managing only a
// marked block. Idempotent.
func InstallHook(dir string) (string, error) {
	common, err := CommonDir(dir)
	if err != nil {
		return "", err
	}
	hooksDir := filepath.Join(common, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return "", err
	}
	hookPath := filepath.Join(hooksDir, "post-checkout")

	existing, err := os.ReadFile(hookPath)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}

	var content string
	if len(existing) == 0 {
		content = "#!/bin/sh\n" + hookBlock() + "\n"
	} else {
		content = replaceBlock(string(existing), hookBlock())
	}

	if err := os.WriteFile(hookPath, []byte(content), 0o755); err != nil {
		return "", err
	}
	return hookPath, nil
}

// replaceBlock swaps an existing wtdb marked block for a fresh one, or appends it
// if absent.
func replaceBlock(existing, block string) string {
	begin := strings.Index(existing, hookMarkerBegin)
	if begin >= 0 {
		end := strings.Index(existing, hookMarkerEnd)
		if end >= 0 {
			end += len(hookMarkerEnd)
			return existing[:begin] + block + existing[end:]
		}
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return existing + "\n" + block + "\n"
}
