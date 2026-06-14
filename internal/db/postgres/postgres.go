// Package postgres implements the wtdb database adapter for PostgreSQL.
//
// The primary copy path is a server-side `CREATE DATABASE <dest> TEMPLATE <src>`,
// executed over a pure-Go pgx connection (no psql/createdb binary required). If
// that fails (e.g. it cannot momentarily disconnect the source), it falls back to
// piping `pg_dump` into `psql`.
package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/willhackett/wtdb/internal/db"
)

func init() { db.Register(Adapter{}) }

// Adapter is the PostgreSQL implementation of db.Adapter.
type Adapter struct{}

func (Adapter) Scheme() string { return "postgres" }

var nonIdent = regexp.MustCompile(`[^a-z0-9_]+`)

// SanitizeDBName lowercases, replaces runs of invalid characters with "_", and
// ensures the name starts with a letter/underscore so it is a valid, unquoted
// PostgreSQL identifier.
func (Adapter) SanitizeDBName(raw string) string {
	s := nonIdent.ReplaceAllString(strings.ToLower(raw), "_")
	s = strings.Trim(s, "_")
	if s == "" {
		s = "db"
	}
	if c := s[0]; !(c == '_' || (c >= 'a' && c <= 'z')) {
		s = "_" + s
	}
	const maxLen = 63 // PostgreSQL identifier limit
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}

// adminConn rewrites a connection URL to target the "postgres" maintenance
// database, which we connect to in order to issue CREATE/DROP DATABASE.
func adminConn(connStr string) (string, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return "", fmt.Errorf("parsing connection string: %w", err)
	}
	u.Path = "/postgres"
	return u.String(), nil
}

// Copy creates dest as a TEMPLATE copy of src, falling back to dump/restore.
func (a Adapter) Copy(ctx context.Context, connStr, src, dest string) error {
	if err := a.copyTemplate(ctx, connStr, src, dest); err != nil {
		if dumpErr := a.copyDump(ctx, connStr, src, dest); dumpErr != nil {
			return fmt.Errorf("template copy failed (%v); dump/restore fallback failed: %w", err, dumpErr)
		}
	}
	return nil
}

func (a Adapter) copyTemplate(ctx context.Context, connStr, src, dest string) error {
	admin, err := adminConn(connStr)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}
	defer conn.Close(ctx)

	// TEMPLATE requires no other sessions on the source database.
	if err := terminate(ctx, conn, src); err != nil {
		return err
	}
	stmt := fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", quoteIdent(dest), quoteIdent(src))
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	return nil
}

// copyDump uses pg_dump | psql, which works against a live source database.
func (a Adapter) copyDump(ctx context.Context, connStr, src, dest string) error {
	admin, err := adminConn(connStr)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", quoteIdent(dest))); err != nil {
		conn.Close(ctx)
		return fmt.Errorf("create empty database: %w", err)
	}
	conn.Close(ctx)

	srcURL, err := withDBName(connStr, src)
	if err != nil {
		return err
	}
	destURL, err := withDBName(connStr, dest)
	if err != nil {
		return err
	}

	dump := exec.CommandContext(ctx, "pg_dump", "--no-owner", "--no-acl", "-d", srcURL)
	restore := exec.CommandContext(ctx, "psql", "-q", "-d", destURL)
	pipe, err := dump.StdoutPipe()
	if err != nil {
		return err
	}
	restore.Stdin = pipe
	if err := restore.Start(); err != nil {
		return fmt.Errorf("starting psql: %w", err)
	}
	if err := dump.Run(); err != nil {
		_ = restore.Wait()
		return fmt.Errorf("pg_dump: %w", err)
	}
	if err := restore.Wait(); err != nil {
		return fmt.Errorf("psql restore: %w", err)
	}
	return nil
}

// Drop terminates connections to name and drops it if present.
func (Adapter) Drop(ctx context.Context, connStr, name string) error {
	admin, err := adminConn(connStr)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}
	defer conn.Close(ctx)

	if err := terminate(ctx, conn, name); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", quoteIdent(name))); err != nil {
		return fmt.Errorf("drop database: %w", err)
	}
	return nil
}

// terminate ends all other backends connected to dbName so it can be used as a
// template or dropped.
func terminate(ctx context.Context, conn *pgx.Conn, dbName string) error {
	const q = `SELECT pg_terminate_backend(pid) FROM pg_stat_activity
	            WHERE datname = $1 AND pid <> pg_backend_pid()`
	if _, err := conn.Exec(ctx, q, dbName); err != nil {
		return fmt.Errorf("terminating connections to %s: %w", dbName, err)
	}
	return nil
}

func withDBName(connStr, name string) (string, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}

// quoteIdent double-quotes a SQL identifier, escaping embedded quotes.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
