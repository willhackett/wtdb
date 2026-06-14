package env

import "testing"

func TestLookup(t *testing.T) {
	contents := "# comment\nFOO=bar\nexport DATABASE_URL=\"postgres://u:p@localhost:5432/app\"\nBAZ='qux'\n"
	cases := map[string]string{
		"FOO":          "bar",
		"DATABASE_URL": "postgres://u:p@localhost:5432/app",
		"BAZ":          "qux",
	}
	for k, want := range cases {
		got, ok := Lookup(contents, k)
		if !ok || got != want {
			t.Errorf("Lookup(%q) = %q, %v; want %q", k, got, ok, want)
		}
	}
	if _, ok := Lookup(contents, "MISSING"); ok {
		t.Errorf("Lookup(MISSING) should be absent")
	}
}

func TestDBNameFromURL(t *testing.T) {
	got, err := DBNameFromURL("postgres://u:p@host:5432/myapp_dev?sslmode=disable")
	if err != nil || got != "myapp_dev" {
		t.Fatalf("got %q, %v; want myapp_dev", got, err)
	}
	if _, err := DBNameFromURL("postgres://host:5432/"); err == nil {
		t.Errorf("expected error for missing db name")
	}
}

func TestSwapDBName(t *testing.T) {
	got, err := SwapDBName("postgres://u:p@host:5432/myapp_dev?sslmode=disable", "myapp_dev_feat")
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://u:p@host:5432/myapp_dev_feat?sslmode=disable"
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestRewriteVarPreservesFile(t *testing.T) {
	contents := "# header\nFOO=bar\nexport DATABASE_URL=\"postgres://u:p@h:5432/app\"\nBAZ=qux\n"
	newURL := "postgres://u:p@h:5432/app_feat"
	got, err := RewriteVar(contents, "DATABASE_URL", newURL)
	if err != nil {
		t.Fatal(err)
	}
	want := "# header\nFOO=bar\nexport DATABASE_URL=\"postgres://u:p@h:5432/app_feat\"\nBAZ=qux\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRewriteVarUnquoted(t *testing.T) {
	got, err := RewriteVar("DATABASE_URL=postgres://h/app\n", "DATABASE_URL", "postgres://h/app2")
	if err != nil {
		t.Fatal(err)
	}
	if got != "DATABASE_URL=postgres://h/app2\n" {
		t.Errorf("got %q", got)
	}
}

func TestRewriteVarMissing(t *testing.T) {
	if _, err := RewriteVar("FOO=bar\n", "DATABASE_URL", "x"); err == nil {
		t.Errorf("expected error for missing key")
	}
}
