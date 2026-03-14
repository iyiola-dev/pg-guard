package checks

import (
	"go/token"
	"testing"

	pgguard "github.com/iyiola-dev/pg-guard"
)

func pos(file string, line int) token.Position {
	return token.Position{Filename: file, Line: line, Column: 1}
}

func TestCheckUnparameterized(t *testing.T) {
	queries := []pgguard.ExtractedQuery{
		{Pos: pos("a.go", 1), BuildMethod: "sprintf", Parameterized: false, SQL: "SELECT %s"},
		{Pos: pos("a.go", 2), BuildMethod: "concat", Parameterized: false, SQL: "SELECT"},
		{Pos: pos("a.go", 3), BuildMethod: "literal", Parameterized: true, SQL: "SELECT $1"},
		{Pos: pos("a.go", 4), BuildMethod: "unknown"},
	}

	findings := CheckUnparameterized(queries)
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}
	for _, f := range findings {
		if f.Check != pgguard.CheckUnparameterizedQuery {
			t.Errorf("expected check %q, got %q", pgguard.CheckUnparameterizedQuery, f.Check)
		}
		if f.Severity != pgguard.SeverityError {
			t.Errorf("expected severity error, got %q", f.Severity)
		}
	}
}

func TestCheckUnparameterized_NoFindings(t *testing.T) {
	queries := []pgguard.ExtractedQuery{
		{BuildMethod: "literal", Parameterized: true, SQL: "SELECT $1"},
		{BuildMethod: "unknown"},
	}
	findings := CheckUnparameterized(queries)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestCheckMissingContext(t *testing.T) {
	queries := []pgguard.ExtractedQuery{
		{Pos: pos("a.go", 1), FuncName: "Query", HasContext: false},
		{Pos: pos("a.go", 2), FuncName: "QueryContext", HasContext: true},
		{Pos: pos("a.go", 3), FuncName: "Exec", HasContext: false},
	}

	findings := CheckMissingContext(queries)
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}
	for _, f := range findings {
		if f.Check != pgguard.CheckMissingContext {
			t.Errorf("expected check %q, got %q", pgguard.CheckMissingContext, f.Check)
		}
		if f.Severity != pgguard.SeverityWarning {
			t.Errorf("expected severity warning, got %q", f.Severity)
		}
	}
}

func TestCheckMissingContext_AllContext(t *testing.T) {
	queries := []pgguard.ExtractedQuery{
		{FuncName: "QueryContext", HasContext: true},
		{FuncName: "ExecContext", HasContext: true},
	}
	findings := CheckMissingContext(queries)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestCheckNPlusOne(t *testing.T) {
	queries := []pgguard.ExtractedQuery{
		{Pos: pos("a.go", 1), InLoop: true, SQL: "SELECT 1"},
		{Pos: pos("a.go", 2), InLoop: false, SQL: "SELECT 2"},
		{Pos: pos("a.go", 3), InLoop: true, SQL: "SELECT 3"},
	}

	findings := CheckNPlusOne(queries)
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}
	for _, f := range findings {
		if f.Check != pgguard.CheckNPlusOne {
			t.Errorf("expected check %q, got %q", pgguard.CheckNPlusOne, f.Check)
		}
		if f.Severity != pgguard.SeverityWarning {
			t.Errorf("expected severity warning, got %q", f.Severity)
		}
	}
}

func TestCheckNPlusOne_NoLoop(t *testing.T) {
	queries := []pgguard.ExtractedQuery{
		{InLoop: false},
	}
	findings := CheckNPlusOne(queries)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestCheckInvalidSQL_Valid(t *testing.T) {
	queries := []pgguard.ExtractedQuery{
		{SQL: "SELECT id FROM users WHERE id = $1", BuildMethod: "literal"},
		{SQL: "INSERT INTO users (name) VALUES ($1)", BuildMethod: "literal"},
	}
	findings := CheckInvalidSQL(queries)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for valid SQL, got %d", len(findings))
	}
}

func TestCheckInvalidSQL_Invalid(t *testing.T) {
	queries := []pgguard.ExtractedQuery{
		{Pos: pos("a.go", 1), SQL: "SLECT id FROM users", BuildMethod: "literal"},
		{Pos: pos("a.go", 2), SQL: "SELECT * FORM users", BuildMethod: "literal"},
	}
	findings := CheckInvalidSQL(queries)
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings for invalid SQL, got %d", len(findings))
	}
	for _, f := range findings {
		if f.Check != pgguard.CheckInvalidSQL {
			t.Errorf("expected check %q, got %q", pgguard.CheckInvalidSQL, f.Check)
		}
		if f.Severity != pgguard.SeverityError {
			t.Errorf("expected severity error, got %q", f.Severity)
		}
	}
}

func TestCheckInvalidSQL_SkipsEmpty(t *testing.T) {
	queries := []pgguard.ExtractedQuery{
		{SQL: "", BuildMethod: "unknown"},
		{SQL: "", BuildMethod: "literal"},
	}
	findings := CheckInvalidSQL(queries)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for empty SQL, got %d", len(findings))
	}
}
