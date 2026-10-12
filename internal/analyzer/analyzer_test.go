package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pgguard "github.com/iyiola-dev/pg-guard"
	"github.com/iyiola-dev/pg-guard/internal/pgtest"
)

func TestMain(m *testing.M) {
	code := m.Run()
	pgtest.Stop()
	os.Exit(code)
}

type result struct {
	file  string
	check pgguard.CheckName
	text  string // part of the message
}

func run(t *testing.T, cfg pgguard.Config) []result {
	t.Helper()
	cfg.Patterns = []string{"./testdata/sample"}
	findings, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results := make([]result, len(findings))
	for i, f := range findings {
		results[i] = result{filepath.Base(f.Pos.Filename), f.Check, f.Message}
	}
	return results
}

func expect(t *testing.T, got, want []result) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d findings, want %d:\n%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].file != want[i].file || got[i].check != want[i].check || !strings.Contains(got[i].text, want[i].text) {
			t.Errorf("finding %d = %v, want %v", i, got[i], want[i])
		}
	}
}

var staticFindings = []result{
	{"sample.go", pgguard.CheckMissingContext, "Query() called without context"},
	{"sample.go", pgguard.CheckUnparameterizedQuery, "fmt.Sprintf"},
	{"sample.go", pgguard.CheckMissingContext, "Query() called without context"},
}

func TestRun_StaticOnly(t *testing.T) {
	expect(t, run(t, pgguard.Config{}), staticFindings)
}

func TestRun_Tests(t *testing.T) {
	want := append([]result{}, staticFindings...)
	want = append(want, result{"sample_test.go", pgguard.CheckInvalidSQL, `syntax error at or near "SELEC"`})
	expect(t, run(t, pgguard.Config{Tests: true}), want)
}

func TestRun_WithDB(t *testing.T) {
	dsn := pgtest.Database(t, "postgres:16-alpine")
	pgtest.Exec(t, dsn,
		`CREATE TABLE users (id serial PRIMARY KEY, name text NOT NULL, email text NOT NULL)`,
		`INSERT INTO users (name, email) SELECT 'user' || i, 'user' || i || '@example.com' FROM generate_series(1, 120000) AS i`,
		`CREATE TABLE orders (id serial PRIMARY KEY, user_id int NOT NULL)`,
		`INSERT INTO orders (user_id) SELECT i % 1000 FROM generate_series(1, 110000) AS i`,
		`ANALYZE`,
	)

	want := append([]result{}, staticFindings...)
	want = append(want,
		result{"sample.go", pgguard.CheckMissingIndex, "sequential scan on public.users (~120k rows) filtering on email"},
		result{"sample.go", pgguard.CheckMissingIndex, "filtering on name"},
		result{"sample.go", pgguard.CheckInvalidSchema, `column "nme" does not exist`},
		result{"sample.go", pgguard.CheckInvalidSchema, "table missing_table does not exist"},
		result{"sample.go", pgguard.CheckFullTableScan, "query reads all of public.orders (~110k rows)"},
	)
	expect(t, run(t, pgguard.Config{DSN: dsn}), want)
}

func TestRun_UnreachableDB(t *testing.T) {
	_, err := Run(context.Background(), pgguard.Config{
		Patterns: []string{"./testdata/sample"},
		DSN:      "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=2",
	})
	if err == nil {
		t.Fatal("expected an error when the database is unreachable")
	}
}
