// Package cli dispatches wtdb subcommands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/willhackett/wtdb/internal/app"
	"github.com/willhackett/wtdb/internal/config"
	"github.com/willhackett/wtdb/internal/git"
)

// Version is set via -ldflags at build time.
var Version = "dev"

const usage = `wtdb — isolated databases for git worktrees

Usage:
  wtdb init       Scaffold a .wtdb.json in the current repo
  wtdb install    Install the git post-checkout hook (auto-provision on worktree add)
  wtdb sync       Provision the current worktree now (copy db, env, paths)
  wtdb prune      Drop databases whose worktrees have been removed
  wtdb status     List worktrees and their databases
  wtdb version    Print the version

Enable a repo by adding .wtdb.json with: envFile, dbEnvVar, pathsToCopy.`

// Main runs the CLI and returns a process exit code.
func Main(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stdout, usage)
		return 0
	}
	ctx := context.Background()
	cwd, err := os.Getwd()
	if err != nil {
		return fail(err)
	}

	switch args[1] {
	case "init":
		return run(cmdInit(cwd, os.Stdout))
	case "install":
		return run(cmdInstall(cwd, os.Stdout))
	case "hook":
		return run(cmdHook(ctx, cwd, args[2:]))
	case "sync":
		force := hasFlag(args[2:], "--force")
		return run(app.Provision(ctx, cwd, force, false, os.Stdout))
	case "prune":
		return run(app.Prune(ctx, cwd, os.Stdout))
	case "status":
		return run(app.Status(cwd, os.Stdout))
	case "version", "--version", "-v":
		fmt.Fprintln(os.Stdout, "wtdb "+Version)
		return 0
	case "help", "--help", "-h":
		fmt.Fprintln(os.Stdout, usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s\n", args[1], usage)
		return 2
	}
}

// cmdInit scaffolds a starter .wtdb.json at the repo root.
func cmdInit(cwd string, out io.Writer) error {
	root, err := git.Toplevel(cwd)
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}
	path := root + string(os.PathSeparator) + config.FileName
	if config.Exists(root) {
		fmt.Fprintf(out, "%s already exists\n", config.FileName)
		return nil
	}
	const starter = `{
  "envFile": ".env",
  "dbEnvVar": "DATABASE_URL",
  "pathsToCopy": ["node_modules"]
}
`
	if err := os.WriteFile(path, []byte(starter), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "Created %s — edit it, then run `wtdb install`.\n", config.FileName)
	return nil
}

// cmdInstall installs the post-checkout hook.
func cmdInstall(cwd string, out io.Writer) error {
	root, err := git.Toplevel(cwd)
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}
	hookPath, err := git.InstallHook(cwd)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Installed git hook: %s\n", hookPath)
	if !config.Exists(root) {
		fmt.Fprintf(out, "Note: no %s found yet — run `wtdb init` to create one.\n", config.FileName)
	}
	fmt.Fprintln(out, "New worktrees (`git worktree add ...`) will now be provisioned automatically.")
	return nil
}

// cmdHook is invoked by the git post-checkout hook. Git passes:
//
//	$1 = previous HEAD, $2 = new HEAD, $3 = branch-checkout flag
//
// On `git worktree add`, the previous HEAD is the null object id; we use that as
// the signal to provision (other checkouts are no-ops, guarded further by state).
func cmdHook(ctx context.Context, cwd string, args []string) error {
	if len(args) == 0 || args[0] != "post-checkout" {
		return nil
	}
	prevHead := ""
	if len(args) >= 2 {
		prevHead = args[1]
	}
	if !isNullOID(prevHead) {
		return nil // not a fresh worktree/checkout; nothing to do
	}
	// Quiet provisioning: never fail the user's git command on our account.
	if err := app.Provision(ctx, cwd, false, false, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "wtdb: provisioning failed: %v\n", err)
	}
	return nil
}

func isNullOID(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && strings.Trim(s, "0") == ""
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func run(err error) int {
	if err != nil {
		return fail(err)
	}
	return 0
}

func fail(err error) int {
	if errors.Is(err, git.ErrNotRepo) {
		fmt.Fprintln(os.Stderr, "wtdb: not inside a git repository — run this from within your project.")
		return 1
	}
	fmt.Fprintf(os.Stderr, "wtdb: %v\n", err)
	return 1
}
