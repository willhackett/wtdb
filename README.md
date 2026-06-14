# wtdb

Give every git worktree its own database.

## The problem

You use `git worktree` to work on several branches at once. But every worktree
points at the **same** local dev database. The moment one branch runs a migration,
it changes the schema the others rely on — so concurrent branches with diverging
migrations step on each other.

A fresh worktree is also missing everything git ignores: your `.env` / `.dev.vars`,
your `node_modules`. So it isn't even runnable until you copy things across by hand.

## What wtdb does

When you create a worktree, wtdb automatically:

1. **Copies your dev database** into a new one named after the worktree
   (Postgres, via a near-instant `CREATE DATABASE ... TEMPLATE`).
2. **Copies your env file** into the worktree and rewrites the database URL to
   point at the new database.
3. **Copies the paths you list** (e.g. `node_modules`) using fast copy-on-write
   clones.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/willhackett/wtdb/main/install.sh | sh
```

(Homebrew coming later, maybe.)

## Enable it in a repo

Add a `.wtdb.json` to the repo root:

```json
{
  "envFile": ".dev.vars",
  "dbEnvVar": "DATABASE_URL",
  "pathsToCopy": ["node_modules"]
}
```

- `envFile` — the env file to copy and rewrite (`.env`, `.dev.vars`, …).
- `dbEnvVar` — the variable in that file holding your `postgres://…` connection URL.
- `pathsToCopy` — gitignored dirs/files to bring into each new worktree.

Then install the git hook (once per repo):

```sh
wtdb init      # writes a starter .wtdb.json if you don't have one
wtdb install   # installs the post-checkout git hook
```

## Usage

Just use git as normal:

```sh
git worktree add ../myapp-feature
# -> wtdb copies the db, writes the env file, clones node_modules
```

When you remove a worktree, reclaim its database:

```sh
git worktree remove ../myapp-feature
wtdb prune     # drops databases whose worktrees are gone
```

Other commands:

```sh
wtdb sync      # provision the current worktree manually
wtdb status    # list worktrees and their databases
```

## Adapters

wtdb ships with a **PostgreSQL** adapter. The database layer is an interface
(`internal/db`), so support for other databases can be added by implementing
`Copy` / `Drop` for a connection-URL scheme and registering it. PRs welcome.

## License

MIT
