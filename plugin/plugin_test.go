package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"github.com/iyiola-dev/pg-guard/internal/pgtest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestMain(m *testing.M) {
	code := m.Run()
	pgtest.Stop()
	os.Exit(code)
}

func testdata(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// The want comments in testdata/src/lint list every expected diagnostic;
// any other diagnostic fails the test.
func TestAnalyzer(t *testing.T) {
	t.Setenv("PG_GUARD_DSN", "") // static checks only
	analysistest.Run(t, testdata(t), NewAnalyzer(Settings{}),
		"lint/dbsql",
		"lint/literals",
		"lint/resolve",
		"lint/pgxcalls",
		"lint/gormcalls",
		"lint/unrelated",
		"lint/sqlc",
	)
}

func TestAnalyzer_LiveDatabase(t *testing.T) {
	dsn := pgtest.Database(t, "postgres:16-alpine")
	pgtest.Exec(t, dsn,
		`CREATE TABLE users (id serial PRIMARY KEY, name text NOT NULL, email text NOT NULL, city text NOT NULL)`,
		`INSERT INTO users (name, email, city)
			SELECT 'user' || i, 'user' || i || '@example.com', 'city' || (i % 1000)
			FROM generate_series(1, 150000) AS i`,
		`CREATE TABLE orders (id serial PRIMARY KEY, user_id int NOT NULL, amount int NOT NULL)`,
		`INSERT INTO orders (user_id, amount) SELECT (i % 150000) + 1, i FROM generate_series(1, 120000) AS i`,
		`ANALYZE`,
	)

	analysistest.Run(t, testdata(t), NewAnalyzer(Settings{DSN: dsn}), "dblint")

	// The UPDATE in dblint was only planned.
	pgtest.Exec(t, dsn, `DO $$ BEGIN
		IF EXISTS (SELECT 1 FROM users WHERE name = 'b') THEN RAISE 'UPDATE was executed'; END IF;
	END $$`)
}

func TestModulePlugin(t *testing.T) {
	newPlugin, err := register.GetPlugin("pgguard")
	if err != nil {
		t.Fatal(err)
	}

	p, err := newPlugin(map[string]any{"dsn": ""})
	if err != nil {
		t.Fatalf("new plugin: %v", err)
	}
	if mode := p.GetLoadMode(); mode != register.LoadModeTypesInfo {
		t.Errorf("load mode = %q, want %q", mode, register.LoadModeTypesInfo)
	}
	analyzers, err := p.BuildAnalyzers()
	if err != nil {
		t.Fatalf("build analyzers: %v", err)
	}
	if len(analyzers) != 1 || analyzers[0].Name != "pgguard" {
		t.Fatalf("analyzers = %v, want one named pgguard", analyzers)
	}

	if _, err := newPlugin(nil); err != nil {
		t.Errorf("plugin without settings: %v", err)
	}
	if _, err := newPlugin(map[string]any{"unknown": true}); err == nil {
		t.Error("expected an error for an unknown setting")
	}
}
