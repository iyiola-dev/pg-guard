package pgguard

import "go/token"

// Severity is the diagnostic level of a [Finding].
type Severity string

const (
	// SeverityError identifies a correctness or SQL construction problem.
	SeverityError Severity = "error"
	// SeverityWarning identifies a potential runtime risk or incomplete analysis.
	SeverityWarning Severity = "warning"
	// SeverityInfo identifies an opportunity to improve query performance.
	SeverityInfo Severity = "info"
)

// CheckName identifies the analysis that produced a [Finding].
type CheckName string

const (
	// CheckUnparameterizedQuery identifies SQL constructed from dynamic values.
	CheckUnparameterizedQuery CheckName = "unparameterized-query"
	// CheckMissingContext identifies database calls known to omit a context.
	CheckMissingContext CheckName = "missing-context"
	// CheckNPlusOne identifies query execution repeated by a loop.
	CheckNPlusOne CheckName = "n-plus-one"
	// CheckMissingIndex identifies a filtered sequential scan that may benefit
	// from an index.
	CheckMissingIndex CheckName = "missing-index"
	// CheckFullTableScan identifies a query that scans most or all of a large table.
	CheckFullTableScan CheckName = "full-table-scan"
	// CheckInvalidSQL identifies SQL rejected by the PostgreSQL syntax parser.
	CheckInvalidSQL CheckName = "invalid-sql"
	// CheckInvalidSchema identifies references rejected by the connected
	// database's schema.
	CheckInvalidSchema CheckName = "invalid-schema"
	// CheckDBAnalysisFailed identifies a query whose database analysis could
	// not complete.
	CheckDBAnalysisFailed CheckName = "db-analysis-failed"
)

// API identifies the database library a call belongs to.
type API string

const (
	// APIDatabaseSQL identifies database/sql and compatible query interfaces.
	APIDatabaseSQL API = "database/sql"
	// APIPgx identifies pgx and compatible query interfaces.
	APIPgx API = "pgx"
	// APIGORM identifies GORM calls and SQL expressions.
	APIGORM API = "gorm"
)

// CallKind describes what a database call does when it runs.
type CallKind string

const (
	// CallQuery sends a statement to the database.
	CallQuery CallKind = "query"
	// CallPrepare prepares a statement for later execution.
	CallPrepare CallKind = "prepare"
	// CallBuild assembles SQL without executing it, as with pgx Batch.Queue
	// or GORM Where.
	CallBuild CallKind = "build"
)

// SQLKind describes how much of a statement a call's SQL text holds.
type SQLKind string

const (
	// SQLNone indicates a call without SQL text, such as prepared statement execution.
	SQLNone SQLKind = ""
	// SQLStatement indicates a complete SQL statement.
	SQLStatement SQLKind = "statement"
	// SQLFragment indicates part of a statement, such as a GORM Where condition.
	SQLFragment SQLKind = "fragment"
)

// BuildMethod describes how a query's SQL text is produced.
type BuildMethod string

const (
	// BuildConstant indicates construction from constant values, even when
	// control flow obscures the final text.
	BuildConstant BuildMethod = "constant"
	// BuildSprintf indicates fmt.Sprintf with non-constant arguments.
	BuildSprintf BuildMethod = "sprintf"
	// BuildConcat indicates concatenation with non-constant operands.
	BuildConcat BuildMethod = "concat"
	// BuildUnknown indicates that SQL construction could not be determined statically.
	BuildUnknown BuildMethod = "unknown"
)

// ExtractedQuery describes a database call and the SQL known about it statically.
type ExtractedQuery struct {
	// Pos locates the called method or function in the source.
	Pos token.Position
	// TokenPos locates Pos in the file set used for extraction.
	TokenPos token.Pos
	API      API
	// FuncName is the called method or function name, such as QueryContext or Where.
	FuncName string
	Call     CallKind
	// HasContext is false only for calls known to run without a context.
	// A true value also covers SQL builders and calls whose context is unknown.
	HasContext bool
	// InLoop reports whether the call appears in an enclosing loop's body,
	// condition, or post statement.
	InLoop bool

	Kind SQLKind
	// Clause identifies the SQL clause of a fragment, such as WHERE or ORDER BY.
	Clause string
	// SQL contains decoded SQL text or a display template for dynamic SQL.
	// It is empty when the text cannot be determined.
	SQL string
	// NormalizedSQL contains SQL rewritten to PostgreSQL bind syntax when the
	// complete text is known. An empty value disables syntax and database checks.
	NormalizedSQL string
	BuildMethod   BuildMethod
}

// Finding is a diagnostic associated with a database call in the source.
type Finding struct {
	// Pos locates the call for text and JSON reports.
	Pos token.Position
	// TokenPos locates the call in the original file set for analysis diagnostics.
	TokenPos token.Pos
	Severity Severity
	Check    CheckName
	Message  string
	// SQL contains the query text or display template, if available.
	SQL string
	// SuggestedFix describes a possible remedy; it is empty when none is known.
	SuggestedFix string
}

// Config controls package loading, optional database analysis, and report output.
type Config struct {
	// Patterns contains Go package patterns accepted by go/packages.
	Patterns []string
	// DSN enables live database analysis when it contains a PostgreSQL
	// connection string.
	DSN string
	// Format selects the CLI report format; "json" selects JSON output.
	Format string
	// Tests includes _test.go files in package analysis.
	Tests bool
}
