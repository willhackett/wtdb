package postgres

import "testing"

func TestSanitizeDBName(t *testing.T) {
	a := Adapter{}
	cases := map[string]string{
		"myapp_dev_feat-x":   "myapp_dev_feat_x",
		"Feature/Cool Thing": "feature_cool_thing",
		"123start":           "_123start",
		"---":                "db",
		"app__dev":           "app__dev",
	}
	for in, want := range cases {
		if got := a.SanitizeDBName(in); got != want {
			t.Errorf("SanitizeDBName(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestSanitizeDBNameLength(t *testing.T) {
	long := make([]byte, 100)
	for i := range long {
		long[i] = 'a'
	}
	if got := (Adapter{}).SanitizeDBName(string(long)); len(got) != 63 {
		t.Errorf("length = %d; want 63", len(got))
	}
}

func TestQuoteIdent(t *testing.T) {
	if got := quoteIdent(`we"ird`); got != `"we""ird"` {
		t.Errorf("quoteIdent = %q", got)
	}
}

func TestAdminConn(t *testing.T) {
	got, err := adminConn("postgres://u:p@h:5432/app?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if got != "postgres://u:p@h:5432/postgres?sslmode=disable" {
		t.Errorf("adminConn = %q", got)
	}
}
