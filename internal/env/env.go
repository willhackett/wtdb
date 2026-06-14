// Package env reads a value from a dotenv-format file and rewrites the database
// name inside a connection-string variable while preserving the rest of the file.
package env

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Lookup returns the raw (unquoted) value of key in a dotenv file's contents.
func Lookup(contents, key string) (string, bool) {
	sc := bufio.NewScanner(strings.NewReader(contents))
	for sc.Scan() {
		k, v, ok := parseLine(sc.Text())
		if ok && k == key {
			return v, true
		}
	}
	return "", false
}

// LookupFile reads path and returns the value of key.
func LookupFile(path, key string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	v, ok := Lookup(string(data), key)
	return v, ok, nil
}

// parseLine splits "KEY=value" (optionally "export KEY=...") into key and
// unquoted value. Returns ok=false for blanks and comments.
func parseLine(line string) (key, value string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	trimmed = strings.TrimPrefix(trimmed, "export ")
	eq := strings.IndexByte(trimmed, '=')
	if eq < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(trimmed[:eq])
	value = unquote(strings.TrimSpace(trimmed[eq+1:]))
	return key, value, key != ""
}

func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

// DBNameFromURL extracts the database name from a postgres connection URL.
func DBNameFromURL(connStr string) (string, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return "", fmt.Errorf("parsing connection string: %w", err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return "", fmt.Errorf("connection string has no database name")
	}
	return name, nil
}

// SwapDBName returns connStr with its database name replaced by newName,
// preserving credentials, host, port and query params.
func SwapDBName(connStr, newName string) (string, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return "", fmt.Errorf("parsing connection string: %w", err)
	}
	u.Path = "/" + newName
	return u.String(), nil
}

// RewriteVar returns contents with key's value replaced by newValue, touching
// only the matching line and preserving its original quoting style. Returns an
// error if the key is not present.
func RewriteVar(contents, key, newValue string) (string, error) {
	lines := strings.Split(contents, "\n")
	found := false
	for i, line := range lines {
		k, _, ok := parseLine(line)
		if !ok || k != key {
			continue
		}
		lines[i] = rewriteValue(line, newValue)
		found = true
	}
	if !found {
		return "", fmt.Errorf("key %q not found", key)
	}
	return strings.Join(lines, "\n"), nil
}

// rewriteValue rebuilds a single dotenv line with newValue, keeping the original
// "export " prefix, key spacing, and quote character.
func rewriteValue(line, newValue string) string {
	trimmed := strings.TrimSpace(line)
	prefix := ""
	if strings.HasPrefix(trimmed, "export ") {
		prefix = "export "
		trimmed = strings.TrimPrefix(trimmed, "export ")
	}
	eq := strings.IndexByte(trimmed, '=')
	key := strings.TrimSpace(trimmed[:eq])
	oldRaw := strings.TrimSpace(trimmed[eq+1:])

	quote := ""
	if len(oldRaw) >= 2 && (oldRaw[0] == '"' || oldRaw[0] == '\'') && oldRaw[len(oldRaw)-1] == oldRaw[0] {
		quote = string(oldRaw[0])
	}
	return fmt.Sprintf("%s%s=%s%s%s", prefix, key, quote, newValue, quote)
}
