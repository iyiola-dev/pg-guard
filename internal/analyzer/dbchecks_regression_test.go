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
