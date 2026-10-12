package checks

import (
	pgguard "github.com/iyiola-dev/pg-guard"
)

// CheckUnparameterized returns errors for SQL constructed with non-constant
// concatenation or fmt.Sprintf arguments. It does not determine whether the
// interpolated values are untrusted.
func CheckUnparameterized(queries []pgguard.ExtractedQuery) []pgguard.Finding {
	var findings []pgguard.Finding

	for _, q := range queries {
		if q.Kind == pgguard.SQLNone {
			continue
		}

		var how string
		switch q.BuildMethod {
		case pgguard.BuildSprintf:
			how = "fmt.Sprintf with non-constant arguments"
		case pgguard.BuildConcat:
			how = "string concatenation with non-constant values"
		default:
			continue
		}

		fix := "Use $1, $2, ... placeholders and pass values as arguments"
		if q.API == pgguard.APIGORM {
			fix = "Use ? placeholders and pass values as arguments"
		}
		findings = append(findings, pgguard.Finding{
			Pos:          q.Pos,
			TokenPos:     q.TokenPos,
			Severity:     pgguard.SeverityError,
			Check:        pgguard.CheckUnparameterizedQuery,
			Message:      "SQL built with " + how + "; use a parameterized query instead",
			SQL:          q.SQL,
			SuggestedFix: fix,
		})
	}

	return findings
}
