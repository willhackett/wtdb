// Package db defines the database adapter interface and a scheme-keyed registry.
// Adapters implement copying a whole database and dropping one. Postgres ships in
// the box; new adapters register themselves for their connection-URL scheme.
package db

import (
	"context"
	"fmt"
	"net/url"
)

// Adapter copies and drops databases for one connection-string scheme.
type Adapter interface {
	// Scheme is the connection URL scheme this adapter handles, e.g. "postgres".
	Scheme() string
	// SanitizeDBName turns an arbitrary worktree name into a valid database name.
	SanitizeDBName(raw string) string
	// Copy makes dest a full copy of src, using the admin connection conn.
	Copy(ctx context.Context, conn string, src, dest string) error
	// Drop removes the database name using the admin connection conn.
	Drop(ctx context.Context, conn string, name string) error
}

var registry = map[string]Adapter{}

// Register makes an adapter available for its scheme. Called from adapter init().
func Register(a Adapter) {
	registry[a.Scheme()] = a
}

// ForConnString selects the adapter matching a connection string's scheme.
func ForConnString(connStr string) (Adapter, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return nil, fmt.Errorf("parsing connection string: %w", err)
	}
	scheme := u.Scheme
	// postgres URLs appear as both "postgres" and "postgresql".
	if scheme == "postgresql" {
		scheme = "postgres"
	}
	a, ok := registry[scheme]
	if !ok {
		return nil, fmt.Errorf("no adapter for scheme %q (supported: %v)", u.Scheme, schemes())
	}
	return a, nil
}

func schemes() []string {
	out := make([]string, 0, len(registry))
	for s := range registry {
		out = append(out, s)
	}
	return out
}
