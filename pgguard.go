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
	CheckInvalidSQL           CheckName = "invalid-sql"
	CheckInvalidSchema        CheckName = "invalid-schema"
	CheckDBAnalysisFailed     CheckName = "db-analysis-failed"
)

// API identifies the database library a call belongs to.
type API string

const (
	APIDatabaseSQL API = "database/sql"
	APIPgx         API = "pgx"
	APIGORM        API = "gorm"
)

// CallKind describes what a database call does when it runs.
type CallKind string

const (
	CallQuery   CallKind = "query"   // sends a statement to the database
	CallPrepare CallKind = "prepare" // prepares a statement for later use
	CallBuild   CallKind = "build"   // only assembles SQL, e.g. pgx Batch.Queue or GORM Where
)

// SQLKind describes how much of a statement a call's SQL text holds.
type SQLKind string

const (
	SQLNone      SQLKind = ""          // no SQL text, e.g. stmt.Query or GORM First without conditions
	SQLStatement SQLKind = "statement" // a complete statement
	SQLFragment  SQLKind = "fragment"  // part of a statement, e.g. a GORM Where condition
)

// BuildMethod describes how a query's SQL text is produced.
type BuildMethod string

const (
	BuildConstant BuildMethod = "constant" // only from compile-time constants
	BuildSprintf  BuildMethod = "sprintf"  // fmt.Sprintf with non-constant arguments
	BuildConcat   BuildMethod = "concat"   // concatenation with non-constant operands
	BuildUnknown  BuildMethod = "unknown"  // not determinable statically
)

type ExtractedQuery struct {
	Pos      token.Position
	TokenPos token.Pos // Pos in the FileSet the query was extracted from
	API      API
	FuncName string // method called, e.g. "QueryContext", "Where"
	Call     CallKind
	// HasContext is false only when the call is known to run without a
	// context.Context.
	HasContext bool
	InLoop     bool

	Kind   SQLKind
	Clause string // clause of a fragment, e.g. "WHERE" or "ORDER BY"
	// SQL is the query text as written, with Go escapes decoded. For
	// dynamic SQL it is a template showing non-constant parts as {expr}.
	SQL string
	// NormalizedSQL is SQL with bind placeholders rewritten to $1, $2, ...
	// It is set only when the whole text is known at compile time.
	NormalizedSQL string
	BuildMethod   BuildMethod
}

type Finding struct {
	Pos          token.Position
	TokenPos     token.Pos
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
