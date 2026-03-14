package checks

import (
	"fmt"

	pgguard "github.com/iyiola-dev/pg-guard"
	pgquery "github.com/pganalyze/pg_query_go/v5"
)

func CheckInvalidSQL(queries []pgguard.ExtractedQuery) []pgguard.Finding {
	var findings []pgguard.Finding

	for _, q := range queries {
		if q.SQL == "" || q.BuildMethod == "unknown" {
			continue
		}

		normalized := normalizePlaceholders(q.SQL)
		_, err := pgquery.Parse(normalized)
		if err != nil {
			findings = append(findings, pgguard.Finding{
				Pos:      q.Pos,
				Severity: pgguard.SeverityError,
				Check:    pgguard.CheckInvalidSQL,
				Message:  fmt.Sprintf("invalid SQL syntax: %v", err),
				SQL:      q.SQL,
			})
		}
	}

	return findings
}

func normalizePlaceholders(sql string) string {
	// pg_query_go handles $1-style placeholders natively, no normalization needed
	return sql
}
