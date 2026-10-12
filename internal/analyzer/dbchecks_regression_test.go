package analyzer

import (
	"context"
	"go/token"
	"strings"
	"testing"

	pgguard "github.com/iyiola-dev/pg-guard"
	"github.com/iyiola-dev/pg-guard/internal/dbinfo"
	"github.com/iyiola-dev/pg-guard/internal/pgtest"
)

func regressionDBChecker(t *testing.T, statements ...string) *DBChecker {
	t.Helper()
	dsn := pgtest.Database(t, "postgres:16-alpine")
	pgtest.Exec(t, dsn, statements...)
	ctx := context.Background()
	db, err := dbinfo.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(func() { db.Close(ctx) })
	return NewDBChecker(db)
}

func regressionQuery(sql string, line int) pgguard.ExtractedQuery {
	return pgguard.ExtractedQuery{
		Pos:           token.Position{Filename: "queries.go", Line: line, Column: 1},
		Kind:          pgguard.SQLStatement,
		SQL:           sql,
		NormalizedSQL: sql,
		BuildMethod:   pgguard.BuildConstant,
	}
}

func regressionCheck(t *testing.T, checker *DBChecker, queries ...pgguard.ExtractedQuery) []pgguard.Finding {
	t.Helper()
	findings, err := checker.Check(context.Background(), queries)
	if err != nil {
		t.Fatalf("check queries: %v", err)
	}
	return findings
}

func TestDBChecker_MissingTable(t *testing.T) {
	checker := regressionDBChecker(t)
	query := regressionQuery("SELECT id FROM regression_missing_table", 10)
	findings := regressionCheck(t, checker, query)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want one missing-table finding: %+v", len(findings), findings)
	}
	finding := findings[0]
	if finding.Check != pgguard.CheckInvalidSchema || finding.Severity != pgguard.SeverityError {
		t.Errorf("got %s/%s, want invalid-schema/error", finding.Check, finding.Severity)
	}
	if !strings.Contains(finding.Message, "regression_missing_table") {
		t.Errorf("missing-table message does not identify the table: %q", finding.Message)
	}
	if finding.Pos != query.Pos || finding.SQL != query.SQL {
		t.Errorf("finding does not identify the original call site: %+v", finding)
	}
}

func TestDBChecker_NestedScan(t *testing.T) {
	checker := regressionDBChecker(t,
		`CREATE TABLE regression_nested_scan (id int NOT NULL, sort_value int NOT NULL)`,
		`INSERT INTO regression_nested_scan SELECT i, 120001 - i FROM generate_series(1, 120000) AS i`,
		`ANALYZE regression_nested_scan`,
	)
	query := regressionQuery("SELECT id FROM regression_nested_scan ORDER BY sort_value", 20)
	findings := regressionCheck(t, checker, query)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want one scan beneath Sort: %+v", len(findings), findings)
	}
	finding := findings[0]
	if finding.Check != pgguard.CheckFullTableScan || finding.Severity != pgguard.SeverityInfo {
		t.Errorf("got %s/%s, want full-table-scan/info", finding.Check, finding.Severity)
	}
	if finding.Pos != query.Pos || finding.SQL != query.SQL {
		t.Errorf("finding does not identify the original call site: %+v", finding)
	}
}

func TestDBChecker_EveryQuery(t *testing.T) {
	checker := regressionDBChecker(t,
		`CREATE TABLE regression_every_query (id int PRIMARY KEY, payload text NOT NULL)`,
		`INSERT INTO regression_every_query SELECT i, 'row-' || i FROM generate_series(1, 120000) AS i`,
		`ANALYZE regression_every_query`,
	)
	indexed := regressionQuery("SELECT payload FROM regression_every_query WHERE id = 1", 10)
	scan := regressionQuery("SELECT payload FROM regression_every_query", 20)
	sameSQLAtAnotherSite := regressionQuery(scan.SQL, 30)

	cases := []struct {
		name    string
		queries []pgguard.ExtractedQuery
		want    []pgguard.ExtractedQuery
	}{
		{"indexed first", []pgguard.ExtractedQuery{indexed, scan}, []pgguard.ExtractedQuery{scan}},
		{"scan first", []pgguard.ExtractedQuery{scan, indexed}, []pgguard.ExtractedQuery{scan}},
		{"same SQL at separate call sites", []pgguard.ExtractedQuery{indexed, scan, sameSQLAtAnotherSite}, []pgguard.ExtractedQuery{scan, sameSQLAtAnotherSite}},
		{"repeat checks retain call sites", []pgguard.ExtractedQuery{sameSQLAtAnotherSite, indexed, scan}, []pgguard.ExtractedQuery{sameSQLAtAnotherSite, scan}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := regressionCheck(t, checker, tc.queries...)
			if len(findings) != len(tc.want) {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), len(tc.want), findings)
			}
			for i, finding := range findings {
				if finding.Check != pgguard.CheckFullTableScan || finding.Severity != pgguard.SeverityInfo {
					t.Errorf("finding %d = %s/%s, want full-table-scan/info", i, finding.Check, finding.Severity)
				}
				if finding.Pos != tc.want[i].Pos || finding.SQL != tc.want[i].SQL {
					t.Errorf("finding %d does not identify the expected call site: got %+v, want %+v", i, finding, tc.want[i])
				}
			}
		})
	}
}

func TestDBChecker_ReportsDatabaseErrors(t *testing.T) {
	checker := regressionDBChecker(t, `CREATE TABLE regression_errors (id int)`)
	cases := []struct {
		name, sql string
		check     pgguard.CheckName
		severity  pgguard.Severity
	}{
		{"invalid column", "SELECT missing FROM regression_errors", pgguard.CheckInvalidSchema, pgguard.SeverityError},
		{"planning failure", "SELECT 1 / 0 FROM regression_errors", pgguard.CheckDBAnalysisFailed, pgguard.SeverityWarning},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := regressionQuery(tc.sql, 40)
			findings := regressionCheck(t, checker, query)
			if len(findings) != 1 || findings[0].Check != tc.check || findings[0].Severity != tc.severity {
				t.Fatalf("got %+v, want %s/%s", findings, tc.check, tc.severity)
			}
			if findings[0].SQL != query.SQL || findings[0].Pos != query.Pos {
				t.Errorf("finding does not identify the original query: %+v", findings[0])
			}
		})
	}
}

func TestDBChecker_ReturnsConnectionErrors(t *testing.T) {
	checker := regressionDBChecker(t)
	if err := checker.db.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := checker.Check(context.Background(), []pgguard.ExtractedQuery{regressionQuery("SELECT 1", 50)})
	if err == nil {
		t.Fatal("expected a connection error instead of a clean result")
	}
}
