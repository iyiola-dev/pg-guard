package dbinfo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

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

// SeqScans returns the sequential scan at the root of the plan, if any.
func (r *ExplainResult) SeqScans() []SeqScan {
	p := r.Plan
	if p == nil || p.NodeType != "Seq Scan" {
		return nil
	}
	return []SeqScan{{Schema: p.Schema, Relation: p.RelationName, Filter: p.Filter, Rows: p.PlanRows}}
}

// DB inspects a live database. Table names passed to its methods are SQL
// names, optionally schema-qualified and quoted, resolved through the search
// path. It is not safe for concurrent use.
type DB struct {
	conn *pgx.Conn
}

func New(ctx context.Context, dsn string) (*DB, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("dbinfo: connect: %w", err)
	}
	return &DB{conn: conn}, nil
}

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

func (db *DB) TableExists(ctx context.Context, table string) (bool, error) {
	var exists bool
	err := db.conn.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("dbinfo: table exists %q: %w", table, err)
	}
	return exists, nil
}

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

// Explain returns the root plan for query without running the statement.
func (db *DB) Explain(ctx context.Context, query string) (*ExplainResult, error) {
	var planJSON []byte
	if err := db.conn.QueryRow(ctx, "EXPLAIN (FORMAT JSON, VERBOSE) "+query).Scan(&planJSON); err != nil {
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
