package app_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/willhackett/wtdb/internal/app"
	"github.com/willhackett/wtdb/internal/env"
	"github.com/willhackett/wtdb/internal/git"
	"github.com/willhackett/wtdb/internal/state"
	"github.com/willhackett/wtdb/internal/testutil"

	// Register the postgres adapter for these tests.
	_ "github.com/willhackett/wtdb/internal/db/postgres"
)

// --- Postgres test harness ---------------------------------------------------

// requirePostgres returns a base admin connection URL (without a database path),
// skipping the test if no server is reachable. Override with WTDB_TEST_PG_URL.
func requirePostgres(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping postgres integration test in -short mode")
	}
	base := os.Getenv("WTDB_TEST_PG_URL")
	if base == "" {
		u, err := user.Current()
		if err != nil {
			t.Skip("no WTDB_TEST_PG_URL and cannot determine current user")
		}
		base = "postgres://" + u.Username + "@localhost:5432"
	}
	base = strings.TrimRight(base, "/")

	conn, err := pgx.Connect(context.Background(), base+"/postgres")
	if err != nil {
		t.Skipf("postgres not available (%s): %v", base, err)
	}
	_ = conn.Close(context.Background())
	return base
}

func adminExec(t *testing.T, base, sql string) {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), base+"/postgres")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(context.Background(), sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func dropDB(t *testing.T, base, name string) {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), base+"/postgres")
	if err != nil {
		return
	}
	defer conn.Close(context.Background())
	_, _ = conn.Exec(context.Background(),
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1 AND pid<>pg_backend_pid()`, name)
	_, _ = conn.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q`, name))
}

func dbExists(t *testing.T, base, name string) bool {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), base+"/postgres")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var n int
	err = conn.QueryRow(context.Background(), `SELECT count(*) FROM pg_database WHERE datname=$1`, name).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func queryScalar(t *testing.T, connURL, sql string) string {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), connURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var v string
	if err := conn.QueryRow(context.Background(), sql).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// seedSource creates a fresh source database with a row to copy, registering
// cleanup, and returns its connection URL.
func seedSource(t *testing.T, base string) (name, connURL string) {
	t.Helper()
	name = "wtdb_test_" + sanitize(t.Name()) + "_src"
	dropDB(t, base, name)
	adminExec(t, base, fmt.Sprintf("CREATE DATABASE %q", name))
	t.Cleanup(func() { dropDB(t, base, name) })

	connURL = base + "/" + name
	c, err := pgx.Connect(context.Background(), connURL)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(context.Background())
	if _, err := c.Exec(context.Background(), "CREATE TABLE widget(id int); INSERT INTO widget VALUES (42)"); err != nil {
		t.Fatal(err)
	}
	return name, connURL
}

// setupRepo builds a repo whose .wtdb.json/.env point at connURL, with a
// committed .gitignore and a node_modules dir to copy.
func setupRepo(t *testing.T, connURL string) string {
	t.Helper()
	repo := testutil.InitRepo(t)
	writeFile(t, filepath.Join(repo, ".env"),
		fmt.Sprintf("DATABASE_URL=\"%s\"\nOTHER=keepme\n", connURL))
	writeFile(t, filepath.Join(repo, ".wtdb.json"),
		`{"envFile":".env","dbEnvVar":"DATABASE_URL","pathsToCopy":["node_modules"]}`)
	if err := os.MkdirAll(filepath.Join(repo, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "node_modules", "marker.txt"), "lib\n")
	writeFile(t, filepath.Join(repo, ".gitignore"), ".env\nnode_modules/\n.wtdb.json\n")
	testutil.Git(t, repo, "add", ".gitignore")
	testutil.Git(t, repo, "commit", "-q", "-m", "ignore")
	return repo
}

// destNameFromWorktree reads the provisioned database name out of the worktree's
// rewritten env file.
func destNameFromWorktree(t *testing.T, worktree string) string {
	t.Helper()
	v, ok, err := env.LookupFile(filepath.Join(worktree, ".env"), "DATABASE_URL")
	if err != nil || !ok {
		t.Fatalf("reading worktree env: ok=%v err=%v", ok, err)
	}
	name, err := env.DBNameFromURL(v)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

// --- Tests -------------------------------------------------------------------

func TestProvisionAndPrune(t *testing.T) {
	base := requirePostgres(t)
	src, conn := seedSource(t, base)
	repo := setupRepo(t, conn)
	wt := testutil.AddWorktree(t, repo, "feat")

	if err := app.Provision(context.Background(), wt, false, true, io.Discard); err != nil {
		t.Fatalf("Provision: %v", err)
	}

	dest := destNameFromWorktree(t, wt)
	t.Cleanup(func() { dropDB(t, base, dest) })

	if dest == src {
		t.Fatalf("destination db equals source %q", src)
	}
	if !dbExists(t, base, dest) {
		t.Fatalf("destination db %q does not exist", dest)
	}
	// Data copied via TEMPLATE.
	if got := queryScalar(t, base+"/"+dest, "SELECT id FROM widget"); got != "42" {
		t.Errorf("widget row = %q; want 42", got)
	}
	// Env rewrite preserves other vars.
	envBody := readFile(t, filepath.Join(wt, ".env"))
	if !strings.Contains(envBody, "OTHER=keepme") {
		t.Errorf("env lost OTHER var:\n%s", envBody)
	}
	// Paths copied.
	if _, err := os.Stat(filepath.Join(wt, "node_modules", "marker.txt")); err != nil {
		t.Errorf("node_modules not copied: %v", err)
	}
	// State recorded.
	common, _ := git.CommonDir(repo)
	st, _ := state.Load(common)
	if len(st.Entries) != 1 || st.Entries[0].DBName != dest {
		t.Errorf("state = %+v; want one entry for %s", st.Entries, dest)
	}

	// Prune while the worktree still exists is a no-op.
	if err := app.Prune(context.Background(), repo, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !dbExists(t, base, dest) {
		t.Errorf("prune dropped a live worktree's db")
	}

	// Remove the worktree, then prune drops the orphaned db.
	testutil.Git(t, repo, "worktree", "remove", "--force", wt)
	if err := app.Prune(context.Background(), repo, io.Discard); err != nil {
		t.Fatal(err)
	}
	if dbExists(t, base, dest) {
		t.Errorf("prune did not drop orphaned db %q", dest)
	}
}

func TestProvisionIdempotent(t *testing.T) {
	base := requirePostgres(t)
	_, conn := seedSource(t, base)
	repo := setupRepo(t, conn)
	wt := testutil.AddWorktree(t, repo, "twice")

	if err := app.Provision(context.Background(), wt, false, true, io.Discard); err != nil {
		t.Fatalf("first Provision: %v", err)
	}
	dest := destNameFromWorktree(t, wt)
	t.Cleanup(func() { dropDB(t, base, dest) })

	// Second run must be a safe no-op (no "database already exists" error).
	if err := app.Provision(context.Background(), wt, false, true, io.Discard); err != nil {
		t.Fatalf("second Provision should be a no-op: %v", err)
	}
	if !dbExists(t, base, dest) {
		t.Errorf("db %q missing after idempotent re-run", dest)
	}
}

func TestProvisionSkipsMainWorktree(t *testing.T) {
	base := requirePostgres(t)
	_, conn := seedSource(t, base)
	repo := setupRepo(t, conn)

	// Running against the main worktree must not create any database.
	if err := app.Provision(context.Background(), repo, false, true, io.Discard); err != nil {
		t.Fatalf("Provision on main: %v", err)
	}
	common, _ := git.CommonDir(repo)
	st, _ := state.Load(common)
	if len(st.Entries) != 0 {
		t.Errorf("main worktree should not be provisioned: %+v", st.Entries)
	}
}

// TestHookFiresOnWorktreeAdd is the true git-integration test: it builds the
// binary, installs the hook via the binary, and verifies a plain
// `git worktree add` provisions the database automatically.
func TestHookFiresOnWorktreeAdd(t *testing.T) {
	base := requirePostgres(t)
	bin := buildBinary(t)
	binDir := filepath.Dir(bin)

	_, conn := seedSource(t, base)
	repo := setupRepo(t, conn)

	// Install the hook through the real CLI.
	runBin(t, bin, repo, binDir, "install")

	// Plain git worktree add — the hook should fire and provision.
	wt := filepath.Join(t.TempDir(), "hooked")
	cmd := exec.Command("git", "worktree", "add", wt, "-b", "hooked")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	dest := destNameFromWorktree(t, wt)
	t.Cleanup(func() { dropDB(t, base, dest) })
	if !dbExists(t, base, dest) {
		t.Fatalf("hook did not provision db %q", dest)
	}
	if got := queryScalar(t, base+"/"+dest, "SELECT id FROM widget"); got != "42" {
		t.Errorf("widget row via hook = %q; want 42", got)
	}
}

// --- helpers -----------------------------------------------------------------

func buildBinary(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Join(filepath.Dir(file), "..", "..")
	bin := filepath.Join(t.TempDir(), "wtdb")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = moduleRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building wtdb: %v\n%s", err, out)
	}
	return bin
}

func runBin(t *testing.T, bin, dir, binDir string, args ...string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", bin, strings.Join(args, " "), err, out)
	}
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
