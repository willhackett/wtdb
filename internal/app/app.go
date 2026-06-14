// Package app implements wtdb's high-level flows: provisioning a worktree's
// database + files, pruning orphaned databases, and reporting status.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/willhackett/wtdb/internal/config"
	"github.com/willhackett/wtdb/internal/db"
	"github.com/willhackett/wtdb/internal/env"
	"github.com/willhackett/wtdb/internal/git"
	"github.com/willhackett/wtdb/internal/pathcopy"
	"github.com/willhackett/wtdb/internal/state"
)

// Provision runs the create flow for the worktree at dir: copy the database,
// rewrite the env file, and clone configured paths. It is idempotent — an
// already-provisioned worktree is skipped unless force is set. quiet suppresses
// per-step output (used by the git hook).
func Provision(ctx context.Context, dir string, force, quiet bool, out io.Writer) error {
	logf := logger(out, quiet)

	main, err := git.MainWorktree(dir)
	if err != nil {
		return err
	}
	top, err := git.Toplevel(dir)
	if err != nil {
		return err
	}
	if top == main {
		// The main worktree IS the source database; nothing to provision.
		logf("wtdb: skipping main worktree")
		return nil
	}

	cfg, err := config.Load(main)
	if err != nil {
		if os.IsNotExist(err) {
			logf("wtdb: no %s in repo root; skipping", config.FileName)
			return nil
		}
		return err
	}

	common, err := git.CommonDir(dir)
	if err != nil {
		return err
	}
	st, err := state.Load(common)
	if err != nil {
		return err
	}
	if _, exists := st.Get(top); exists && !force {
		logf("wtdb: worktree already provisioned; skipping (use --force to redo)")
		return nil
	}

	// Read the source connection string from the main worktree's env file.
	srcConn, ok, err := env.LookupFile(filepath.Join(main, cfg.EnvFile), cfg.DBEnvVar)
	if err != nil {
		return fmt.Errorf("reading %s: %w", cfg.EnvFile, err)
	}
	if !ok {
		return fmt.Errorf("%s not found in %s", cfg.DBEnvVar, cfg.EnvFile)
	}
	srcDB, err := env.DBNameFromURL(srcConn)
	if err != nil {
		return err
	}

	adapter, err := db.ForConnString(srcConn)
	if err != nil {
		return err
	}
	destDB := destDBName(adapter, srcDB, top)

	logf("wtdb: copying database %s -> %s ...", srcDB, destDB)
	dbStart := time.Now()
	if err := adapter.Copy(ctx, srcConn, srcDB, destDB); err != nil {
		return fmt.Errorf("copying database: %w", err)
	}
	logf("wtdb: database ready (%s)", dur(dbStart))

	st.Upsert(state.Entry{WorktreePath: top, DBName: destDB, SourceDB: srcDB})
	if err := st.Save(); err != nil {
		return err
	}

	if err := writeEnvFile(main, top, cfg, srcConn, destDB); err != nil {
		return fmt.Errorf("writing env file: %w", err)
	}
	logf("wtdb: wrote %s pointing at %s", cfg.EnvFile, destDB)

	for _, p := range cfg.PathsToCopy {
		logf("wtdb: copying %s ...", p)
		start := time.Now()
		method, copied, err := pathcopy.Copy(filepath.Join(main, p), filepath.Join(top, p))
		if err != nil {
			return fmt.Errorf("copying %s: %w", p, err)
		}
		switch {
		case !copied:
			logf("wtdb: skipped %s (missing or already present)", p)
		case method == "clone":
			logf("wtdb: copied %s (cloned, %s)", p, dur(start))
		default:
			logf("wtdb: copied %s (%s, %s)", p, method, dur(start))
		}
	}

	logf("wtdb: ready — %s isolated on database %s", filepath.Base(top), destDB)
	return nil
}

// destDBName derives a sanitized "<srcDB>_<worktree>" database name.
func destDBName(a db.Adapter, srcDB, worktreePath string) string {
	return a.SanitizeDBName(srcDB + "_" + filepath.Base(worktreePath))
}

// writeEnvFile copies the source env file into the worktree and rewrites the DB
// var to point at destDB.
func writeEnvFile(main, worktree string, cfg *config.Config, srcConn, destDB string) error {
	data, err := os.ReadFile(filepath.Join(main, cfg.EnvFile))
	if err != nil {
		return err
	}
	newConn, err := env.SwapDBName(srcConn, destDB)
	if err != nil {
		return err
	}
	rewritten, err := env.RewriteVar(string(data), cfg.DBEnvVar, newConn)
	if err != nil {
		return err
	}
	dest := filepath.Join(worktree, cfg.EnvFile)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, []byte(rewritten), 0o600)
}

// Prune drops databases whose worktrees no longer exist.
func Prune(ctx context.Context, dir string, out io.Writer) error {
	logf := logger(out, false)

	main, err := git.MainWorktree(dir)
	if err != nil {
		return err
	}
	cfg, err := config.Load(main)
	if err != nil {
		return err
	}
	srcConn, ok, err := env.LookupFile(filepath.Join(main, cfg.EnvFile), cfg.DBEnvVar)
	if err != nil || !ok {
		return fmt.Errorf("reading %s from %s: %w", cfg.DBEnvVar, cfg.EnvFile, err)
	}
	adapter, err := db.ForConnString(srcConn)
	if err != nil {
		return err
	}

	common, err := git.CommonDir(dir)
	if err != nil {
		return err
	}
	st, err := state.Load(common)
	if err != nil {
		return err
	}
	wts, err := git.ListWorktrees(dir)
	if err != nil {
		return err
	}
	live := map[string]bool{}
	for _, w := range wts {
		live[w.Path] = true
	}

	dropped := 0
	for _, e := range append([]state.Entry(nil), st.Entries...) {
		if live[e.WorktreePath] {
			continue
		}
		logf("wtdb: dropping orphaned database %s (worktree %s gone)", e.DBName, filepath.Base(e.WorktreePath))
		if err := adapter.Drop(ctx, srcConn, e.DBName); err != nil {
			return fmt.Errorf("dropping %s: %w", e.DBName, err)
		}
		st.Remove(e.WorktreePath)
		dropped++
	}
	if err := st.Save(); err != nil {
		return err
	}
	if dropped == 0 {
		logf("wtdb: nothing to prune")
	} else {
		logf("wtdb: pruned %d database(s)", dropped)
	}
	return nil
}

// Status prints the worktrees wtdb has provisioned and their databases. Worktrees
// without a wtdb database are omitted to keep the output focused.
func Status(dir string, out io.Writer) error {
	common, err := git.CommonDir(dir)
	if err != nil {
		return err
	}
	st, err := state.Load(common)
	if err != nil {
		return err
	}
	wts, err := git.ListWorktrees(dir)
	if err != nil {
		return err
	}
	live := map[string]bool{}
	for _, w := range wts {
		live[w.Path] = true
	}

	shown := 0
	for _, e := range st.Entries {
		if !live[e.WorktreePath] {
			continue // orphaned; `wtdb prune` will clean it up
		}
		fmt.Fprintf(out, "%-50s  %s\n", filepath.Base(e.WorktreePath), e.DBName)
		shown++
	}
	if shown == 0 {
		fmt.Fprintln(out, "No worktrees provisioned yet.")
		fmt.Fprintln(out, "Add one with `git worktree add ...` (hook installed) or run `wtdb sync` inside a worktree.")
	}
	return nil
}

// dur formats elapsed time since start compactly (e.g. "0.2s", "63s").
func dur(start time.Time) string {
	d := time.Since(start)
	if d < time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.0fs", d.Seconds())
}

func logger(out io.Writer, quiet bool) func(string, ...any) {
	return func(format string, args ...any) {
		if quiet {
			return
		}
		fmt.Fprintf(out, format+"\n", args...)
	}
}
