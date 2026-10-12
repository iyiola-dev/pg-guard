package extractor

import (
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/tools/go/packages"
)

func loadTestPackages(t *testing.T, dir string) []*packages.Package {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedFiles | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  dir,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatalf("failed to load packages: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages loaded")
	}
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			t.Fatalf("package errors: %v", pkg.Errors)
		}
	}
	return pkgs
}

func testdataDir(sub string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", sub)
}

func TestExtract_Basic(t *testing.T) {
	pkgs := loadTestPackages(t, testdataDir("basic"))
	queries := Extract(pkgs)

	if len(queries) == 0 {
		t.Fatal("expected queries to be extracted, got 0")
	}

	var foundLiteral, foundSprintf, foundConcat, foundContext, foundLoop bool
	for _, q := range queries {
		t.Logf("func=%s build=%s param=%v ctx=%v loop=%v sql=%q",
			q.FuncName, q.BuildMethod, q.Parameterized, q.HasContext, q.InLoop, q.SQL)

		switch {
		case q.FuncName == "Query" && q.BuildMethod == "literal" && !q.InLoop:
			foundLiteral = true
			if !q.Parameterized {
				t.Error("literal query with $1 should be marked parameterized")
			}
		case q.FuncName == "Query" && q.BuildMethod == "sprintf":
			foundSprintf = true
			if q.Parameterized {
				t.Error("sprintf query should not be marked parameterized")
			}
		case q.FuncName == "Query" && q.BuildMethod == "concat":
			foundConcat = true
			if q.Parameterized {
				t.Error("concat query should not be marked parameterized")
			}
		case q.FuncName == "QueryContext":
			foundContext = true
			if !q.HasContext {
				t.Error("QueryContext should have HasContext=true")
			}
		case q.FuncName == "Query" && q.InLoop:
			foundLoop = true
		}
	}

	if !foundLiteral {
		t.Error("did not find literal query")
	}
	if !foundSprintf {
		t.Error("did not find sprintf query")
	}
	if !foundConcat {
		t.Error("did not find concat query")
	}
	if !foundContext {
		t.Error("did not find context query")
	}
	if !foundLoop {
		t.Error("did not find loop query")
	}
}

func TestExtract_ReceiverAndSQLArgument(t *testing.T) {
	queries := Extract(loadTestPackages(t, testdataDir("receivers")))

	type expected struct {
		method     string
		hasContext bool
	}
	want := map[string]expected{
		"SELECT 'db-query'":                        {"Query", false},
		"SELECT 'db-exec'":                         {"Exec", false},
		"SELECT 'db-query-row'":                    {"QueryRow", false},
		"SELECT 'db-query-context'":                {"QueryContext", true},
		"SELECT 'db-exec-context'":                 {"ExecContext", true},
		"SELECT 'db-query-row-context'":            {"QueryRowContext", true},
		"SELECT 'tx-query'":                        {"Query", false},
		"SELECT 'tx-exec-context'":                 {"ExecContext", true},
		"SELECT 'conn-query-context'":              {"QueryContext", true},
		"SELECT 'pgx-query'":                       {"Query", true},
		"SELECT 'pgx-exec'":                        {"Exec", true},
		"SELECT 'pgx-query-row'":                   {"QueryRow", true},
		"SELECT 'pgx-tx-query'":                    {"Query", true},
		"SELECT 'embedded-query'":                  {"Query", false},
		"SELECT 'method-expression-query'":         {"Query", false},
		"SELECT 'method-expression-query-context'": {"QueryContext", true},
	}
	if len(queries) != len(want) {
		t.Fatalf("got %d SQL calls, want %d: %+v", len(queries), len(want), queries)
	}
	seen := make(map[string]bool)
	for _, query := range queries {
		expectation, ok := want[query.SQL]
		if !ok {
			t.Errorf("unexpected SQL %q from %s; unrelated methods and Stmt values must be ignored", query.SQL, query.FuncName)
			continue
		}
		if seen[query.SQL] {
			t.Errorf("duplicate extraction of %q", query.SQL)
		}
		seen[query.SQL] = true
		if query.FuncName != expectation.method || query.HasContext != expectation.hasContext || query.BuildMethod != "literal" {
			t.Errorf("%q: got method=%s context=%v build=%s, want method=%s context=%v build=literal", query.SQL, query.FuncName, query.HasContext, query.BuildMethod, expectation.method, expectation.hasContext)
		}
	}
	for sql := range want {
		if !seen[sql] {
			t.Errorf("missing SQL call %q", sql)
		}
	}
}
