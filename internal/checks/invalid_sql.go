package checks

import (
	"fmt"

	pgguard "github.com/iyiola-dev/pg-guard"
	"github.com/iyiola-dev/pg-guard/internal/sqltext"
	pgquery "github.com/pganalyze/pg_query_go/v5"
)

func CheckInvalidSQL(queries []pgguard.ExtractedQuery) []pgguard.Finding {
	var findings []pgguard.Finding

	for _, q := range queries {
		if q.NormalizedSQL == "" || q.Kind == pgguard.SQLNone {
			continue
		}

		sql, msg := q.NormalizedSQL, "invalid SQL syntax: %v"
		if q.Kind == pgguard.SQLFragment {
			sql, msg = sqltext.Statement(sql, q.Clause), "invalid SQL syntax in "+q.Clause+" fragment: %v"
		}
		if _, err := pgquery.Parse(sql); err != nil {
			findings = append(findings, pgguard.Finding{
				Pos:      q.Pos,
				TokenPos: q.TokenPos,
				Severity: pgguard.SeverityError,
				Check:    pgguard.CheckInvalidSQL,
				Message:  fmt.Sprintf(msg, err),
				SQL:      q.SQL,
			})
		}
	}

	return findings
}
