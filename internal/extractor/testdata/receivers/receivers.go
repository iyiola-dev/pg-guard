package receivers

import (
	"context"
	stdsql "database/sql"

	"github.com/jackc/pgx/v5"
)

// Like Gin's request Query method, this reads a request parameter, not SQL.
type requestContext struct{}

func (*requestContext) Query(key string) string   { return key }
func (*requestContext) Exec(value string) string  { return value }
func (*requestContext) Where(value string) string { return value }

type embeddedDB struct{ *stdsql.DB }

func calls(ctx context.Context, db *stdsql.DB, tx *stdsql.Tx, conn *stdsql.Conn, stmt *stdsql.Stmt, pg *pgx.Conn, pgTx pgx.Tx, wrapped *embeddedDB, request *requestContext) {
	db.Query("SELECT 'db-query'")
	db.Exec("SELECT 'db-exec'")
	db.QueryRow("SELECT 'db-query-row'")
	db.QueryContext(ctx, "SELECT 'db-query-context'")
	db.ExecContext(ctx, "SELECT 'db-exec-context'")
	db.QueryRowContext(ctx, "SELECT 'db-query-row-context'")

	tx.Query("SELECT 'tx-query'")
	tx.ExecContext(ctx, "SELECT 'tx-exec-context'")
	conn.QueryContext(ctx, "SELECT 'conn-query-context'")

	pg.Query(ctx, "SELECT 'pgx-query'")
	pg.Exec(ctx, "SELECT 'pgx-exec'")
	pg.QueryRow(ctx, "SELECT 'pgx-query-row'")
	pgTx.Query(ctx, "SELECT 'pgx-tx-query'")

	wrapped.Query("SELECT 'embedded-query'")
	(*stdsql.DB).Query(db, "SELECT 'method-expression-query'")
	(*stdsql.DB).QueryContext(db, ctx, "SELECT 'method-expression-query-context'")

	// These are bound values for a prepared statement, never query text.
	stmt.Query("bound value")
	stmt.Exec("bound value")
	stmt.QueryRow("bound value")
	stmt.QueryContext(ctx, "bound value")
	stmt.ExecContext(ctx, "bound value")
	stmt.QueryRowContext(ctx, "bound value")

	request.Query("name")
	request.Exec("value")
	request.Where("filter")
}
