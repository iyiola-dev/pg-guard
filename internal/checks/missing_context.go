package checks

import (
	pgguard "github.com/iyiola-dev/pg-guard"
)

func CheckMissingContext(queries []pgguard.ExtractedQuery) []pgguard.Finding {
	var findings []pgguard.Finding

	for _, q := range queries {
		if q.HasContext {
			continue
		}

		findings = append(findings, pgguard.Finding{
			Pos:          q.Pos,
			Severity:     pgguard.SeverityWarning,
			Check:        pgguard.CheckMissingContext,
			Message:      q.FuncName + "() called without context; use " + q.FuncName + "Context()",
			SQL:          q.SQL,
			SuggestedFix: "Use " + q.FuncName + "Context() with a context.Context",
		})
	}

	return findings
}
