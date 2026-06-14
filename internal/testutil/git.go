// Package testutil provides helpers for tests that drive real git repositories.
// It is imported only by test files, so it is never linked into the wtdb binary.
package testutil

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Git runs a git command in dir and returns trimmed stdout, failing the test on
// error.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// InitRepo creates a temp git repo with an initial commit and returns its path.
func InitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	Git(t, dir, "init", "-q", "-b", "main")
	Git(t, dir, "config", "user.email", "test@example.com")
	Git(t, dir, "config", "user.name", "test")
	if err := writeFile(filepath.Join(dir, "README.md"), "# test\n"); err != nil {
		t.Fatal(err)
	}
	Git(t, dir, "add", "README.md")
	Git(t, dir, "commit", "-q", "-m", "init")
	return dir
}

// AddWorktree runs `git worktree add` for a fresh branch and returns the new
// worktree path.
func AddWorktree(t *testing.T, repo, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	Git(t, repo, "worktree", "add", "-q", path, "-b", name)
	return path
}
