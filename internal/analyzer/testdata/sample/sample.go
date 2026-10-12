package sample

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func unsafeQuery(db *sql.DB, name string) {
	db.Query(fmt.Sprintf("SELECT id FROM users WHERE name = '%s'", name))
}

func noContextQuery(db *sql.DB) {
	db.Query("SELECT id FROM users WHERE id = $1", 1)
}

func pgxQueries(ctx context.Context, conn *pgx.Conn) {
	conn.QueryRow(ctx, "SELECT id FROM users WHERE email = $1", "a@example.com")
	conn.QueryRow(ctx, "SELECT id FROM users WHERE name = $1 ORDER BY id", "a")
	conn.Exec(ctx, "SELECT nme FROM users")
	conn.Query(ctx, "SELECT id FROM missing_table")
	conn.Query(ctx, "SELECT user_id, count(*) FROM orders GROUP BY user_id")
}
