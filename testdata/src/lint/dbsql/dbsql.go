package dbsql

import (
	"context"
	"database/sql"
	"fmt"
)

func noContext(db *sql.DB, tx *sql.Tx) {
	db.Query("SELECT id FROM users WHERE id = $1", 1)    // want `\[missing-context\] Query\(\) called without context; use QueryContext\(\)`
	db.QueryRow("SELECT id FROM users WHERE id = $1", 1) // want `\[missing-context\] QueryRow\(\) called without context`
	tx.Exec("DELETE FROM users WHERE id = $1", 1)        // want `\[missing-context\] Exec\(\) called without context`
	db.Prepare("SELECT name FROM users WHERE id = $1")   // want `\[missing-context\] Prepare\(\) called without context`
}

func withContext(ctx context.Context, db *sql.DB, tx *sql.Tx, conn *sql.Conn) {
	db.QueryContext(ctx, "SELECT id FROM users WHERE id = $1", 1)
	tx.ExecContext(ctx, "UPDATE users SET name = $1 WHERE id = $2", "a", 1)
	conn.QueryRowContext(ctx, "SELECT name FROM users WHERE id = $1", 1)
	db.PingContext(ctx)
}

func invalid(ctx context.Context, db *sql.DB) {
	db.QueryContext(ctx, "SELEC id FROM users")                 // want `\[invalid-sql\] invalid SQL syntax: syntax error at or near "SELEC"`
	db.PrepareContext(ctx, "SELECT name FROM users WHERE id =") // want `\[invalid-sql\] invalid SQL syntax: syntax error at end of input`
}

func prepared(ctx context.Context, db *sql.DB, ids []int) {
	stmt, _ := db.PrepareContext(ctx, "SELECT name FROM users WHERE id = $1")
	stmt.QueryRowContext(ctx, 1)
	stmt.ExecContext(ctx, "alice")
	stmt.Query(1) // want `\[missing-context\] Query\(\) called without context`
	for _, id := range ids {
		stmt.QueryContext(ctx, id) // want `\[n-plus-one\] query executed inside loop body`
	}
}

func loops(ctx context.Context, db *sql.DB, ids []int) {
	for _, id := range ids {
		db.QueryRowContext(ctx, "SELECT name FROM users WHERE id = $1", id) // want `\[n-plus-one\] query executed inside loop body`
	}
	for i := 0; i < len(ids); i++ {
		func() {
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", ids[i]) // want `\[n-plus-one\]`
		}()
	}
	for rows, _ := db.QueryContext(ctx, "SELECT id FROM users"); rows.Next(); {
	}
}

func injection(ctx context.Context, db *sql.DB, name, table string) {
	db.QueryContext(ctx, fmt.Sprintf("SELECT id FROM users WHERE name = '%s'", name)) // want `\[unparameterized-query\] SQL built with fmt.Sprintf with non-constant arguments`
	db.QueryContext(ctx, "SELECT id FROM users WHERE name = '"+name+"'")              // want `\[unparameterized-query\] SQL built with string concatenation with non-constant values`
	db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s WHERE id = $1", table), 1)     // want `\[unparameterized-query\] SQL built with fmt.Sprintf`
}
