package analyzer

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	pgguard "github.com/iyiola-dev/pg-guard"
	"github.com/iyiola-dev/pg-guard/internal/dbinfo"
	"github.com/iyiola-dev/pg-guard/internal/sqltext"
)

const largeTableThreshold = 100_000

// DBChecker checks queries against a live database. It caches table
// metadata across calls, and is safe for concurrent use.
type DBChecker struct {
	db *dbinfo.DB

	mu     sync.Mutex
	tables map[string]*tableMeta
}

type tableMeta struct {
	exists  bool
	rows    int64
	indexes []dbinfo.IndexInfo
	err     error // Postgres rejected the metadata query
}

type planOutcome struct {
	result *dbinfo.ExplainResult
	err    error // Postgres rejected the EXPLAIN
}

func NewDBChecker(db *dbinfo.DB) *DBChecker {
	return &DBChecker{
		db:     db,
		tables: make(map[string]*tableMeta),
	}
}

// Check validates the tables each query uses and inspects its plan.
// Queries that Postgres cannot inspect are skipped.
func (c *DBChecker) Check(ctx context.Context, queries []pgguard.ExtractedQuery) ([]pgguard.Finding, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	type candidate struct {
		q    pgguard.ExtractedQuery
		info *sqltext.Info
	}
	var candidates []candidate
	// Tables the code creates itself, such as temporary tables, have nothing
	// in the database to check against.
	created := make(map[string]bool)
	for _, q := range queries {
		if q.Kind != pgguard.SQLStatement || q.NormalizedSQL == "" {
			continue
		}
		info, err := sqltext.Inspect(q.NormalizedSQL)
		if err != nil {
			continue // reported by invalid-sql
		}
		for _, t := range info.Created {
			created[t.Name] = true
		}
		if info.Statements == 1 && info.Command != "" {
			candidates = append(candidates, candidate{q, info})
		}
	}

	var findings []pgguard.Finding
	seenTables := make(map[sqltext.Table]bool)
	for _, cand := range candidates {
		if slices.ContainsFunc(cand.info.Tables, func(t sqltext.Table) bool { return created[t.Name] }) {
			continue
		}
		// Check only the first query referring to each table.
		unseen := len(cand.info.Tables) == 0
		for _, table := range cand.info.Tables {
			if !seenTables[table] {
				unseen = true
			}
			seenTables[table] = true
		}
		if !unseen {
			continue
		}
		fs, err := c.checkQuery(ctx, cand.q, cand.info)
		if err != nil {
			return nil, err
		}
		findings = append(findings, fs...)
	}
	return findings, nil
}

func (c *DBChecker) checkQuery(ctx context.Context, q pgguard.ExtractedQuery, info *sqltext.Info) ([]pgguard.Finding, error) {
	var findings []pgguard.Finding
	for _, t := range info.Tables {
		meta, err := c.table(ctx, t)
		if err != nil {
			return nil, err
		}
		switch {
		case meta.err != nil:
			return nil, nil
		case !meta.exists:
			findings = append(findings, finding(q, pgguard.SeverityError, pgguard.CheckInvalidSchema,
				fmt.Sprintf("table %s does not exist", t), ""))
		}
	}
	if len(findings) > 0 {
		return findings, nil
	}

	plan, err := c.plan(ctx, q.NormalizedSQL)
	if err != nil {
		return nil, err
	}
	if plan.err != nil {
		return nil, nil
	}

	type scanKey struct {
		table  sqltext.Table
		filter string
	}
	reported := make(map[scanKey]bool)
	for _, scan := range plan.result.SeqScans() {
		t := sqltext.Table{Schema: scan.Schema, Name: scan.Relation}
		key := scanKey{t, scan.Filter}
		if reported[key] {
			continue
		}
		reported[key] = true

		meta, err := c.table(ctx, t)
		if err != nil {
			return nil, err
		}
		if meta.err != nil || !meta.exists || meta.rows < largeTableThreshold {
			continue
		}
		if f, ok := scanFinding(q, t, scan, meta); ok {
			findings = append(findings, f)
		}
	}
	return findings, nil
}

func (c *DBChecker) table(ctx context.Context, t sqltext.Table) (*tableMeta, error) {
	name := t.String()
	if meta, ok := c.tables[name]; ok {
		return meta, nil
	}

	meta := &tableMeta{}
	var err error
	meta.exists, err = c.db.TableExists(ctx, name)
	if err == nil && meta.exists {
		meta.rows, err = c.db.TableRowEstimate(ctx, name)
	}
	if err == nil && meta.rows >= largeTableThreshold {
		meta.indexes, err = c.db.TableIndexes(ctx, name)
	}
	if err != nil {
		meta.err = err
	}
	c.tables[name] = meta
	return meta, nil
}

func (c *DBChecker) plan(ctx context.Context, sql string) (*planOutcome, error) {
	result, err := c.db.Explain(ctx, sql)
	return &planOutcome{result: result, err: err}, nil
}

func scanFinding(q pgguard.ExtractedQuery, t sqltext.Table, scan dbinfo.SeqScan, meta *tableMeta) (pgguard.Finding, bool) {
	size := formatRows(meta.rows)
	// A filter that keeps most of the table cannot use an index well.
	if scan.Filter == "" || scan.Rows >= float64(meta.rows)/2 {
		if scan.Limited {
			return pgguard.Finding{}, false
		}
		return finding(q, pgguard.SeverityInfo, pgguard.CheckFullTableScan,
			fmt.Sprintf("query reads all of %s (~%s rows) with a sequential scan", t, size), ""), true
	}

	cols := sqltext.FilterColumns(scan.Filter)
	if len(cols) == 0 {
		return finding(q, pgguard.SeverityInfo, pgguard.CheckMissingIndex,
			fmt.Sprintf("sequential scan on %s (~%s rows) with filter %s; consider adding an index", t, size, scan.Filter),
			"Add an index that supports the filter"), true
	}
	// An index led by a filtered column exists, so the planner chose the
	// scan on purpose, e.g. because the filter is not selective.
	for _, idx := range meta.indexes {
		if len(idx.Columns) > 0 && slices.Contains(cols, idx.Columns[0]) {
			return pgguard.Finding{}, false
		}
	}
	list := strings.Join(cols, ", ")
	return finding(q, pgguard.SeverityInfo, pgguard.CheckMissingIndex,
		fmt.Sprintf("sequential scan on %s (~%s rows) filtering on %s; consider an index", t, size, list),
		fmt.Sprintf("Consider an index on %s (%s)", t, list)), true
}

func finding(q pgguard.ExtractedQuery, sev pgguard.Severity, check pgguard.CheckName, msg, fix string) pgguard.Finding {
	return pgguard.Finding{
		Pos:          q.Pos,
		TokenPos:     q.TokenPos,
		Severity:     sev,
		Check:        check,
		Message:      msg,
		SQL:          q.SQL,
		SuggestedFix: fix,
	}
}

func formatRows(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	}
	return strconv.FormatInt(n, 10)
}
