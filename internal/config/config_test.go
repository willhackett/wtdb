package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{"envFile":".dev.vars","dbEnvVar":"DATABASE_URL","pathsToCopy":["node_modules",".wrangler"]}`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.EnvFile != ".dev.vars" || c.DBEnvVar != "DATABASE_URL" || len(c.PathsToCopy) != 2 {
		t.Errorf("unexpected config: %+v", c)
	}
}

func TestLoadValidation(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{"envFile":"",  "dbEnvVar":"X"}`)
	if _, err := Load(dir); err == nil {
		t.Errorf("expected validation error for empty envFile")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	if Exists(dir) {
		t.Errorf("Exists should be false in empty dir")
	}
	write(t, dir, `{"envFile":".env","dbEnvVar":"DATABASE_URL"}`)
	if !Exists(dir) {
		t.Errorf("Exists should be true after writing config")
	}
}

func write(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
