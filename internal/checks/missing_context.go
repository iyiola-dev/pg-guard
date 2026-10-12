package checks

import (
	pgguard "github.com/iyiola-dev/pg-guard"
)

// CheckMissingContext returns warnings for calls known to execute or prepare
// statements without a context. Calls that only build SQL are skipped.
func CheckMissingContext(queries []pgguard.ExtractedQuery) []pgguard.Finding {
	var findings []pgguard.Finding

	for _, q := range queries {
		if q.HasContext || (q.Call != pgguard.CallQuery && q.Call != pgguard.CallPrepare) {
			continue
		}

		f := pgguard.Finding{
			Pos:          q.Pos,
			TokenPos:     q.TokenPos,
			Severity:     pgguard.SeverityWarning,
			Check:        pgguard.CheckMissingContext,
			Message:      q.FuncName + "() called without context; use " + q.FuncName + "Context()",
			SQL:          q.SQL,
			SuggestedFix: "Use " + q.FuncName + "Context() with a context.Context",
		}
		if q.API == pgguard.APIGORM {
			f.Message = q.FuncName + "() runs without a context; chain WithContext(ctx) before it"
			f.SuggestedFix = "Call db.WithContext(ctx) at the start of the chain"
		}
		findings = append(findings, f)
	}

	return findings
}
