package checks

import (
	pgguard "github.com/iyiola-dev/pg-guard"
)

func CheckUnparameterized(queries []pgguard.ExtractedQuery) []pgguard.Finding {
	var findings []pgguard.Finding

	for _, q := range queries {
		if q.Parameterized || q.BuildMethod == "literal" || q.BuildMethod == "unknown" {
			continue
		}

		msg := "SQL built with " + q.BuildMethod + "; use parameterized query instead"
		findings = append(findings, pgguard.Finding{
			Pos:          q.Pos,
			Severity:     pgguard.SeverityError,
			Check:        pgguard.CheckUnparameterizedQuery,
			Message:      msg,
			SQL:          q.SQL,
			SuggestedFix: "Use $1, $2, ... placeholders and pass values as arguments",
		})
	}

	return findings
}
