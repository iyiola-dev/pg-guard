package checks

import (
	pgguard "github.com/iyiola-dev/pg-guard"
)

// CheckNPlusOne returns warnings for query execution inside loops. The check
// identifies repeated execution without inferring relationships between queries.
func CheckNPlusOne(queries []pgguard.ExtractedQuery) []pgguard.Finding {
	var findings []pgguard.Finding

	for _, q := range queries {
		if !q.InLoop || q.Call != pgguard.CallQuery {
			continue
		}

		findings = append(findings, pgguard.Finding{
			Pos:          q.Pos,
			TokenPos:     q.TokenPos,
			Severity:     pgguard.SeverityWarning,
			Check:        pgguard.CheckNPlusOne,
			Message:      "query executed inside loop body (N+1 pattern)",
			SQL:          q.SQL,
			SuggestedFix: "Batch the query or use a JOIN to avoid N+1",
		})
	}

	return findings
}
