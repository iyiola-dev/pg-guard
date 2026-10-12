package literals

import (
	"context"
	"database/sql"
)

const columns = "id, name"

const byID = "SELECT " + columns + " FROM users WHERE id = $1"

type Status string

const Active Status = "active"

func escapes(ctx context.Context, db *sql.DB) {
	db.QueryContext(ctx, "SELECT \"name\" FROM users")
	db.QueryContext(ctx, "SELECT id\nFROM users\tWHERE id = $1", 1)
	db.QueryContext(ctx, `SELECT "id" FROM "users"`)
	db.QueryContext(ctx, "SELECT 'it''s' AS quote, '\\d+' AS pattern")
	db.QueryContext(ctx, "SELECT \"name\" FROM \"users") // want `\[invalid-sql\] invalid SQL syntax: unterminated quoted identifier`
}

func constants(ctx context.Context, db *sql.DB) {
	db.QueryContext(ctx, "SELECT id "+"FROM users WHERE id = $1", 1)
	db.QueryContext(ctx, "SELECT "+columns+" FROM users")
	db.QueryContext(ctx, byID, 1)
	db.QueryContext(ctx, "SELECT id FROM users WHERE status = '"+string(Active)+"'")
	db.QueryContext(ctx, byID+" ORDER BY name LIMIT") // want `\[invalid-sql\] invalid SQL syntax: syntax error at end of input`
}
