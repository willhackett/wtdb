package git_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/willhackett/wtdb/internal/git"
	"github.com/willhackett/wtdb/internal/testutil"
)

func TestListAndClassifyWorktrees(t *testing.T) {
	repo := testutil.InitRepo(t)
	wt := testutil.AddWorktree(t, repo, "feature")

	// Resolve symlinks: macOS temp dirs go through /private, and git reports
	// the canonical path.
	repoReal := realpath(t, repo)
	wtReal := realpath(t, wt)

	wts, err := git.ListWorktrees(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 2 {
		t.Fatalf("got %d worktrees; want 2: %+v", len(wts), wts)
	}

	main, err := git.MainWorktree(repo)
	if err != nil {
		t.Fatal(err)
	}
	if main != repoReal {
		t.Errorf("MainWorktree = %q; want %q", main, repoReal)
	}

	// Main worktree is not linked; the added one is.
	if linked, _ := git.IsLinkedWorktree(repo); linked {
		t.Errorf("main worktree reported as linked")
	}
	if linked, err := git.IsLinkedWorktree(wt); err != nil || !linked {
		t.Errorf("linked worktree IsLinkedWorktree = %v, %v; want true", linked, err)
	}

	// The linked worktree's branch is recorded.
	var found bool
	for _, w := range wts {
		if w.Path == wtReal {
			found = true
			if w.Branch != "feature" {
				t.Errorf("branch = %q; want feature", w.Branch)
			}
		}
	}
	if !found {
		t.Errorf("linked worktree %q not in list %+v", wtReal, wts)
	}
}

func TestInstallHookFresh(t *testing.T) {
	repo := testutil.InitRepo(t)
	path, err := git.InstallHook(repo)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("hook is not executable: %v", info.Mode())
	}
	body := read(t, path)
	if !strings.Contains(body, "wtdb hook post-checkout") {
		t.Errorf("hook missing wtdb invocation:\n%s", body)
	}
}

func TestInstallHookIdempotentAndPreserving(t *testing.T) {
	repo := testutil.InitRepo(t)

	// Pre-existing custom hook content must survive.
	hookDir := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(hookDir, "post-checkout")
	custom := "#!/bin/sh\necho custom-hook-line\n"
	if err := os.WriteFile(hookPath, []byte(custom), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := git.InstallHook(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := git.InstallHook(repo); err != nil { // second install must not duplicate
		t.Fatal(err)
	}

	body := read(t, hookPath)
	if !strings.Contains(body, "custom-hook-line") {
		t.Errorf("custom hook content was lost:\n%s", body)
	}
	if n := strings.Count(body, "wtdb hook post-checkout"); n != 1 {
		t.Errorf("wtdb block appears %d times; want 1:\n%s", n, body)
	}
}

func realpath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
