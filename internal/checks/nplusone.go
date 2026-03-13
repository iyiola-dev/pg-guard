package checks

import (
	pgguard "github.com/iyiola-dev/pg-guard"
)

func CheckNPlusOne(queries []pgguard.ExtractedQuery) []pgguard.Finding {
	var findings []pgguard.Finding

	for _, q := range queries {
		if !q.InLoop {
			continue
		}

		findings = append(findings, pgguard.Finding{
			Pos:          q.Pos,
			Severity:     pgguard.SeverityWarning,
			Check:        pgguard.CheckNPlusOne,
			Message:      "query executed inside loop body (N+1 pattern)",
			SQL:          q.SQL,
			SuggestedFix: "Batch the query or use a JOIN to avoid N+1",
		})
	}

	return findings
}
