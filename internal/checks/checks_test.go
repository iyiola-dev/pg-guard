package checks

import (
	"go/token"
	"strings"
	"testing"

	pgguard "github.com/iyiola-dev/pg-guard"
)

func pos(file string, line int) token.Position {
	return token.Position{Filename: file, Line: line, Column: 1}
}

// stmt returns a database/sql query running sql, which is known exactly.
func stmt(line int, sql string) pgguard.ExtractedQuery {
	return pgguard.ExtractedQuery{
		Pos: pos("a.go", line), API: pgguard.APIDatabaseSQL, FuncName: "QueryContext", Call: pgguard.CallQuery,
		HasContext: true, Kind: pgguard.SQLStatement, SQL: sql, NormalizedSQL: sql, BuildMethod: pgguard.BuildConstant,
	}
}

func TestCheckUnparameterized(t *testing.T) {
	sprintf := stmt(1, "SELECT %s")
	sprintf.BuildMethod, sprintf.NormalizedSQL = pgguard.BuildSprintf, ""
	concat := stmt(2, "SELECT id FROM users WHERE name = '{name}'")
	concat.BuildMethod, concat.NormalizedSQL = pgguard.BuildConcat, ""
	gorm := concat
	gorm.Pos, gorm.API, gorm.Kind, gorm.Clause = pos("a.go", 3), pgguard.APIGORM, pgguard.SQLFragment, "WHERE"
	unknown := stmt(4, "")
	unknown.BuildMethod, unknown.NormalizedSQL = pgguard.BuildUnknown, ""

	findings := CheckUnparameterized([]pgguard.ExtractedQuery{sprintf, concat, gorm, unknown, stmt(5, "SELECT $1")})
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings, got %d: %v", len(findings), findings)
	}
	for _, f := range findings {
		if f.Check != pgguard.CheckUnparameterizedQuery || f.Severity != pgguard.SeverityError {
			t.Errorf("unexpected finding %+v", f)
		}
	}
	if !strings.Contains(findings[0].Message, "fmt.Sprintf") || !strings.Contains(findings[1].Message, "concatenation") {
		t.Errorf("messages do not name the build method: %q, %q", findings[0].Message, findings[1].Message)
	}
	if !strings.Contains(findings[2].SuggestedFix, "?") {
		t.Errorf("GORM fix should suggest ? placeholders, got %q", findings[2].SuggestedFix)
	}
}

func TestCheckMissingContext(t *testing.T) {
	query := stmt(1, "SELECT 1")
	query.FuncName, query.HasContext = "Query", false
	prepare := query
	prepare.Pos, prepare.FuncName, prepare.Call = pos("a.go", 2), "Prepare", pgguard.CallPrepare
	gormFind := pgguard.ExtractedQuery{Pos: pos("a.go", 3), API: pgguard.APIGORM, FuncName: "Find", Call: pgguard.CallQuery}
	// Calls that only build SQL never run without a context.
	gormWhere := pgguard.ExtractedQuery{Pos: pos("a.go", 4), API: pgguard.APIGORM, FuncName: "Where", Call: pgguard.CallBuild}

	findings := CheckMissingContext([]pgguard.ExtractedQuery{query, prepare, gormFind, gormWhere, stmt(5, "SELECT 1")})
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings, got %d: %v", len(findings), findings)
	}
	want := []string{
		"Query() called without context; use QueryContext()",
		"Prepare() called without context; use PrepareContext()",
		"Find() runs without a context; chain WithContext(ctx) before it",
	}
	for i, f := range findings {
		if f.Check != pgguard.CheckMissingContext || f.Severity != pgguard.SeverityWarning || f.Message != want[i] {
			t.Errorf("finding %d = %+v, want message %q", i, f, want[i])
		}
	}
}

func TestCheckNPlusOne(t *testing.T) {
	inLoop := stmt(1, "SELECT 1")
	inLoop.InLoop = true
	prepareInLoop := inLoop
	prepareInLoop.Pos, prepareInLoop.Call = pos("a.go", 2), pgguard.CallPrepare
	batchQueue := inLoop
	batchQueue.Pos, batchQueue.Call = pos("a.go", 3), pgguard.CallBuild

	findings := CheckNPlusOne([]pgguard.ExtractedQuery{inLoop, prepareInLoop, batchQueue, stmt(4, "SELECT 2")})
	if len(findings) != 1 || findings[0].Pos.Line != 1 {
		t.Fatalf("expected 1 finding on line 1, got %v", findings)
	}
	if f := findings[0]; f.Check != pgguard.CheckNPlusOne || f.Severity != pgguard.SeverityWarning {
		t.Errorf("unexpected finding %+v", f)
	}
}

func TestCheckInvalidSQL_Valid(t *testing.T) {
	fragment := pgguard.ExtractedQuery{
		Kind: pgguard.SQLFragment, Clause: "WHERE", SQL: "name = ? AND id IN ?",
		NormalizedSQL: "name = $1 AND id IN ($2)", BuildMethod: pgguard.BuildConstant,
	}
	queries := []pgguard.ExtractedQuery{
		stmt(1, "SELECT id FROM users WHERE id = $1"),
		stmt(2, "INSERT INTO users (name) VALUES ($1)"),
		stmt(3, "SELECT \"name\"\nFROM \"users\""),
		fragment,
	}
	if findings := CheckInvalidSQL(queries); len(findings) != 0 {
		t.Fatalf("expected 0 findings for valid SQL, got %v", findings)
	}
}

func TestCheckInvalidSQL_Invalid(t *testing.T) {
	fragment := pgguard.ExtractedQuery{
		Pos: pos("a.go", 3), Kind: pgguard.SQLFragment, Clause: "WHERE", SQL: "name = ? AND",
		NormalizedSQL: "name = $1 AND", BuildMethod: pgguard.BuildConstant,
	}
	queries := []pgguard.ExtractedQuery{
		stmt(1, "SLECT id FROM users"),
		stmt(2, "SELECT * FORM users"),
		fragment,
	}
	findings := CheckInvalidSQL(queries)
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings for invalid SQL, got %d", len(findings))
	}
	for _, f := range findings {
		if f.Check != pgguard.CheckInvalidSQL || f.Severity != pgguard.SeverityError {
			t.Errorf("unexpected finding %+v", f)
		}
	}
	if !strings.Contains(findings[2].Message, "WHERE fragment") {
		t.Errorf("fragment message should name the clause, got %q", findings[2].Message)
	}
}

// Only SQL known exactly is parsed: a Sprintf template or a concatenation
// prefix is not valid SQL on its own.
func TestCheckInvalidSQL_SkipsInexactSQL(t *testing.T) {
	template := stmt(1, "SELECT * FROM %s WHERE id = $1")
	template.BuildMethod, template.NormalizedSQL = pgguard.BuildSprintf, ""
	prefix := stmt(2, "SELECT id FROM users WHERE name = '{name}'")
	prefix.BuildMethod, prefix.NormalizedSQL = pgguard.BuildConcat, ""

	if findings := CheckInvalidSQL([]pgguard.ExtractedQuery{template, prefix, stmt(3, "")}); len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %v", findings)
	}
}
