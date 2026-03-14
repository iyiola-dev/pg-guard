package report

import (
	"bytes"
	"encoding/json"
	"go/token"
	"strings"
	"testing"

	pgguard "github.com/iyiola-dev/pg-guard"
)

func sampleFindings() []pgguard.Finding {
	return []pgguard.Finding{
		{
			Pos:      token.Position{Filename: "main.go", Line: 42, Column: 3},
			Severity: pgguard.SeverityError,
			Check:    pgguard.CheckUnparameterizedQuery,
			Message:  "SQL built with sprintf; use parameterized query instead",
			SQL:      "SELECT %s",
		},
		{
			Pos:      token.Position{Filename: "main.go", Line: 55, Column: 5},
			Severity: pgguard.SeverityWarning,
			Check:    pgguard.CheckMissingContext,
			Message:  "Query() called without context; use QueryContext()",
		},
	}
}

func TestText(t *testing.T) {
	var buf bytes.Buffer
	err := Text(sampleFindings(), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "main.go:42:3") {
		t.Error("expected file:line:col in output")
	}
	if !strings.Contains(out, "error") {
		t.Error("expected severity in output")
	}
	if !strings.Contains(out, "unparameterized-query") {
		t.Error("expected check name in output")
	}
	if !strings.Contains(out, "main.go:55:5") {
		t.Error("expected second finding in output")
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
}

func TestText_Empty(t *testing.T) {
	var buf bytes.Buffer
	err := Text(nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

func TestJSON(t *testing.T) {
	var buf bytes.Buffer
	err := JSON(sampleFindings(), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []jsonFinding
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	first := results[0]
	if first.File != "main.go" {
		t.Errorf("expected file main.go, got %q", first.File)
	}
	if first.Line != 42 {
		t.Errorf("expected line 42, got %d", first.Line)
	}
	if first.Severity != "error" {
		t.Errorf("expected severity error, got %q", first.Severity)
	}
	if first.Check != "unparameterized-query" {
		t.Errorf("expected check unparameterized-query, got %q", first.Check)
	}
	if first.SQL != "SELECT %s" {
		t.Errorf("expected SQL in output, got %q", first.SQL)
	}
}

func TestJSON_Empty(t *testing.T) {
	var buf bytes.Buffer
	err := JSON([]pgguard.Finding{}, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []jsonFinding
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}
