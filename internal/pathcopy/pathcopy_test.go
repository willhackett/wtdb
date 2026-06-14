package pathcopy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyTreeContents(t *testing.T) {
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "a.txt"), "alpha")
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(src, "nested", "b.txt"), "bravo")

	dst := filepath.Join(t.TempDir(), "out")
	method, copied, err := Copy(src, dst)
	if err != nil || !copied {
		t.Fatalf("Copy = %q, %v, %v", method, copied, err)
	}
	// On macOS this should take the instant clonefile path.
	if runtime.GOOS == "darwin" && method != "clone" {
		t.Errorf("on darwin expected method=clone, got %q", method)
	}
	if got := read(t, filepath.Join(dst, "a.txt")); got != "alpha" {
		t.Errorf("a.txt = %q", got)
	}
	if got := read(t, filepath.Join(dst, "nested", "b.txt")); got != "bravo" {
		t.Errorf("nested/b.txt = %q", got)
	}
}

func TestCopyMissingSrcIsNoop(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out")
	method, copied, err := Copy(filepath.Join(t.TempDir(), "nope"), dst)
	if err != nil || copied || method != "" {
		t.Errorf("missing src should be a no-op: %q, %v, %v", method, copied, err)
	}
}

func TestCopyExistingDstIsNoop(t *testing.T) {
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "a.txt"), "x")
	dst := t.TempDir() // already exists
	_, copied, err := Copy(src, dst)
	if err != nil || copied {
		t.Errorf("existing dst should be a no-op: copied=%v err=%v", copied, err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
