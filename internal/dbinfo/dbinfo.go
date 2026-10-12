// Package dbinfo inspects PostgreSQL schema metadata and query plans.
package dbinfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/iyiola-dev/pg-guard/internal/sqltext"
	"github.com/jackc/pgx/v5"
)

// IndexInfo describes an index's columns and uniqueness.
type IndexInfo struct {
	Name    string
	Table   string
	Columns []string
	Unique  bool
}

// PlanNode is a node of a JSON EXPLAIN plan.
type PlanNode struct {
	NodeType       string      `json:"Node Type"`
	RelationName   string      `json:"Relation Name"`
	Schema         string      `json:"Schema"`
	Alias          string      `json:"Alias"`
	Filter         string      `json:"Filter"`
	PlanRows       float64     `json:"Plan Rows"`
	ParallelAware  bool        `json:"Parallel Aware"`
	WorkersPlanned int         `json:"Workers Planned"`
	Plans          []*PlanNode `json:"Plans"`
}

// ExplainResult contains the plan returned by PostgreSQL EXPLAIN.
type ExplainResult struct {
	Plan *PlanNode
}

// SeqScan is a sequential scan found in a plan.
type SeqScan struct {
	Schema   string
	Relation string
	Filter   string
	// Rows is the planner's estimate of the rows the scan returns, summed
	// over parallel workers.
	Rows float64
	// Limited reports that a Limit directly above the scan can stop it
	// before it reads the whole table.
	Limited bool
}

// SeqScans returns every sequential scan in the plan, including those
// beneath sort, aggregate, join and other nodes.
func (r *ExplainResult) SeqScans() []SeqScan {
	var scans []SeqScan
	var walk func(n *PlanNode, limited bool, workers int)
	walk = func(n *PlanNode, limited bool, workers int) {
		if n == nil {
			return
		}
		switch n.NodeType {
		case "Seq Scan":
			rows := n.PlanRows
			if n.ParallelAware {
				rows *= parallelDivisor(workers)
			}
			scans = append(scans, SeqScan{Schema: n.Schema, Relation: n.RelationName, Filter: n.Filter, Rows: rows, Limited: limited})
		case "Limit":
			limited = true
		case "Gather", "Gather Merge":
			workers = n.WorkersPlanned
		case "Result", "Append", "Subquery Scan":
			// These stream rows, so a Limit above them still stops the scan early.
		default:
			limited = false
		}
		for _, child := range n.Plans {
			walk(child, limited, workers)
		}
	}
	walk(r.Plan, false, 0)
	return scans
}

// parallelDivisor mirrors the planner's division of a parallel scan's rows
// between workers and the participating leader.
func parallelDivisor(workers int) float64 {
	return float64(workers) + max(0, 1-0.3*float64(workers))
}

// DB inspects a live database. Table names passed to its methods are SQL
// names, optionally schema-qualified and quoted, resolved through the search
// path. It is not safe for concurrent use.
type DB struct {
	conn    *pgx.Conn
	version int // server_version_num
}

// New connects to the PostgreSQL database identified by dsn.
// The caller must close the returned DB when it is no longer needed.
func New(ctx context.Context, dsn string) (*DB, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("dbinfo: connect: %w", err)
	}
	var version int
	if err := conn.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&version); err != nil {
		conn.Close(ctx)
		return nil, fmt.Errorf("dbinfo: server version: %w", err)
	}
	return &DB{conn: conn, version: version}, nil
}

// Close closes the database connection.
func (db *DB) Close(ctx context.Context) error {
	return db.conn.Close(ctx)
}

// TableRowEstimate estimates a table's rows the way the planner does,
// scaling the density recorded by the last VACUUM or ANALYZE to the
// table's current size. Tables never analyzed fall back to the statistics
// collector's live row count.
func (db *DB) TableRowEstimate(ctx context.Context, table string) (int64, error) {
	var estimate int64
	err := db.conn.QueryRow(ctx, `
		SELECT CASE
			WHEN c.reltuples < 0 OR c.relpages = 0 THEN GREATEST(c.reltuples, COALESCE(s.n_live_tup, 0))
			ELSE c.reltuples / c.relpages * (pg_relation_size(c.oid) / current_setting('block_size')::int)
		END::bigint
		FROM pg_class c
		LEFT JOIN pg_stat_all_tables s ON s.relid = c.oid
		WHERE c.oid = to_regclass($1)
	`, table).Scan(&estimate)
	if err != nil {
		return 0, fmt.Errorf("dbinfo: row estimate for %q: %w", table, err)
	}
	return estimate, nil
}

// TableIndexes returns the indexes on table in index-name order.
// Columns contains the ordinary table columns stored in each index;
// index expressions are omitted.
func (db *DB) TableIndexes(ctx context.Context, table string) ([]IndexInfo, error) {
	rows, err := db.conn.Query(ctx, `
		SELECT i.relname, ix.indisunique, array_agg(a.attname ORDER BY k.n)
		FROM pg_index ix
		JOIN pg_class i ON i.oid = ix.indexrelid
		JOIN unnest(ix.indkey) WITH ORDINALITY AS k(attnum, n) ON true
		JOIN pg_attribute a ON a.attrelid = ix.indrelid AND a.attnum = k.attnum
		WHERE ix.indrelid = to_regclass($1)
		GROUP BY i.relname, ix.indisunique
		ORDER BY i.relname
	`, table)
	if err != nil {
		return nil, fmt.Errorf("dbinfo: indexes for %q: %w", table, err)
	}
	defer rows.Close()

	var indexes []IndexInfo
	for rows.Next() {
		var idx IndexInfo
		idx.Table = table
		if err := rows.Scan(&idx.Name, &idx.Unique, &idx.Columns); err != nil {
			return nil, fmt.Errorf("dbinfo: scan index: %w", err)
		}
		indexes = append(indexes, idx)
	}
	return indexes, rows.Err()
}

// TableExists reports whether table resolves to a relation in the database.
func (db *DB) TableExists(ctx context.Context, table string) (bool, error) {
	var exists bool
	err := db.conn.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("dbinfo: table exists %q: %w", table, err)
	}
	return exists, nil
}

// ColumnExists reports whether table has a column with the given name.
// The column name is matched exactly, without SQL identifier normalization.
func (db *DB) ColumnExists(ctx context.Context, table, column string) (bool, error) {
	var exists bool
	err := db.conn.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_attribute
			WHERE attrelid = to_regclass($1) AND attname = $2 AND attnum > 0 AND NOT attisdropped
		)
	`, table, column).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("dbinfo: column exists %q.%q: %w", table, column, err)
	}
	return exists, nil
}

// Explain returns the plan for a single SELECT, INSERT, UPDATE, DELETE or
// MERGE statement without running it. $N placeholders get a generic plan:
// EXPLAIN (GENERIC_PLAN) on Postgres 16 and later, and a prepared statement
// with plan_cache_mode = force_generic_plan before that. Everything happens
// in a read-only transaction that is rolled back.
func (db *DB) Explain(ctx context.Context, query string) (*ExplainResult, error) {
	info, err := sqltext.Inspect(query)
	if err != nil {
		return nil, fmt.Errorf("dbinfo: explain: %w", err)
	}
	if info.Statements != 1 || info.Command == "" {
		return nil, errors.New("dbinfo: explain: only a single SELECT, INSERT, UPDATE, DELETE or MERGE statement can be explained")
	}

	tx, err := db.conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("dbinfo: explain: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = '30s'; SET LOCAL lock_timeout = '5s'"); err != nil {
		return nil, fmt.Errorf("dbinfo: explain: %w", err)
	}

	var planJSON []byte
	switch {
	case info.Params == 0:
		planJSON, err = queryText(ctx, tx, "EXPLAIN (FORMAT JSON, VERBOSE) "+query)
	case db.version >= 160000:
		planJSON, err = queryText(ctx, tx, "EXPLAIN (FORMAT JSON, VERBOSE, GENERIC_PLAN) "+query)
	default:
		planJSON, err = explainPrepared(ctx, tx, query, info.Params)
	}
	if err != nil {
		return nil, fmt.Errorf("dbinfo: explain: %w", err)
	}

	var plans []struct {
		Plan *PlanNode `json:"Plan"`
	}
	if err := json.Unmarshal(planJSON, &plans); err != nil {
		return nil, fmt.Errorf("dbinfo: parse explain: %w", err)
	}
	if len(plans) == 0 {
		return &ExplainResult{}, nil
	}
	return &ExplainResult{Plan: plans[0].Plan}, nil
}

// explainPrepared plans query generically on servers without GENERIC_PLAN.
// Prepared statements outlive transactions, so the statement is always
// deallocated, recovering the transaction first if EXPLAIN fails.
func explainPrepared(ctx context.Context, tx pgx.Tx, query string, params int) ([]byte, error) {
	if _, err := tx.Exec(ctx, "SET LOCAL plan_cache_mode = force_generic_plan"); err != nil {
		return nil, err
	}
	if _, err := tx.Conn().PgConn().Exec(ctx, "PREPARE pgguard_explain AS "+query).ReadAll(); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "SAVEPOINT pgguard_explain"); err != nil {
		return nil, err
	}

	args := strings.TrimSuffix(strings.Repeat("NULL, ", params), ", ")
	plan, explainErr := queryText(ctx, tx, "EXPLAIN (FORMAT JSON, VERBOSE) EXECUTE pgguard_explain("+args+")")
	if explainErr != nil {
		if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT pgguard_explain"); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, "DEALLOCATE pgguard_explain"); err != nil {
		return nil, err
	}
	return plan, explainErr
}

// queryText runs a statement returning one text value over the simple
// query protocol, so that Postgres reads $N placeholders as part of the
// statement instead of expecting bound arguments.
func queryText(ctx context.Context, tx pgx.Tx, sql string) ([]byte, error) {
	results, err := tx.Conn().PgConn().Exec(ctx, sql).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(results) != 1 || len(results[0].Rows) == 0 || len(results[0].Rows[0]) == 0 {
		return nil, errors.New("unexpected EXPLAIN output")
	}
	return results[0].Rows[0][0], nil
}
