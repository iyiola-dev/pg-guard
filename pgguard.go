package pgguard

import "go/token"

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type CheckName string

const (
	CheckUnparameterizedQuery CheckName = "unparameterized-query"
	CheckMissingContext       CheckName = "missing-context"
	CheckNPlusOne             CheckName = "n-plus-one"
	CheckMissingIndex         CheckName = "missing-index"
	CheckFullTableScan        CheckName = "full-table-scan"
)

type ExtractedQuery struct {
	Pos           token.Position
	EndPos        token.Position
	FuncName      string // e.g. "db.Query", "db.ExecContext"
	SQL           string
	Parameterized bool
	HasContext     bool
	InLoop        bool
	BuildMethod   string // "literal", "sprintf", "concat", "unknown"
	Tables        []string
}

type Finding struct {
	Pos          token.Position
	Severity     Severity
	Check        CheckName
	Message      string
	SQL          string
	SuggestedFix string
}

type Config struct {
	Patterns []string
	DSN      string
	Format   string
}
