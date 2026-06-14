package state

import "testing"

func TestRegistryRoundTrip(t *testing.T) {
	dir := t.TempDir()

	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Upsert(Entry{WorktreePath: "/wt/a", DBName: "app_a", SourceDB: "app"})
	s.Upsert(Entry{WorktreePath: "/wt/b", DBName: "app_b", SourceDB: "app"})
	// Upsert on existing path replaces, not duplicates.
	s.Upsert(Entry{WorktreePath: "/wt/a", DBName: "app_a2", SourceDB: "app"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Entries) != 2 {
		t.Fatalf("got %d entries; want 2", len(reloaded.Entries))
	}
	e, ok := reloaded.Get("/wt/a")
	if !ok || e.DBName != "app_a2" {
		t.Errorf("entry /wt/a = %+v, %v; want db app_a2", e, ok)
	}

	reloaded.Remove("/wt/a")
	if _, ok := reloaded.Get("/wt/a"); ok {
		t.Errorf("entry /wt/a should be removed")
	}
	if len(reloaded.Entries) != 1 {
		t.Errorf("got %d entries after remove; want 1", len(reloaded.Entries))
	}
}

func TestLoadEmpty(t *testing.T) {
	s, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Entries) != 0 {
		t.Errorf("expected empty state")
	}
}
