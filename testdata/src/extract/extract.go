// Package extract holds calls whose extracted details are checked by
// internal/extractor's tests. Each call is marked with an @name comment.
package extract

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"
)

const cols = "id, name"

func cases(ctx context.Context, db *sql.DB, conn *pgx.Conn, gdb *gorm.DB, name string, n int) {
	db.QueryContext(ctx, "SELECT \"name\"\nFROM users")                          // @escaped
	db.QueryContext(ctx, `SELECT "id" FROM "users"`)                             // @backtick
	db.QueryContext(ctx, "SELECT "+cols+" FROM users")                           // @constconcat
	db.QueryContext(ctx, "SELECT id FROM users WHERE name = '"+name+"'")         // @dynconcat
	db.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM users LIMIT %d", cols, 10)) // @constsprintf
	db.QueryContext(ctx, fmt.Sprintf("SELECT id FROM users LIMIT %d", n))        // @dynsprintf
	q := "SELECT id FROM users WHERE id = $1"
	db.QueryContext(ctx, q, 1) // @assigned

	conn.Query(ctx, "SELECT id FROM users WHERE name = @name OR alias = @name", pgx.NamedArgs{"name": name}) // @pgxnamed
	gdb.Where("name = ? AND id IN ? AND note = '?'", name, []int{1}).Find(&[]int{})                          // @gormwhere
	gdb.Raw("SELECT * FROM users WHERE name = @name", sql.Named("name", name)).Scan(&[]int{})                // @gormnamed

	stmt, _ := db.Prepare("SELECT 1") // @prepare
	stmt.Query(1)                     // @stmt

	for _, id := range []int{1, 2} {
		db.QueryContext(ctx, "SELECT $1::int", id) // @rangebody
	}
	for rows, _ := db.QueryContext(ctx, "SELECT 1"); rows.Next(); { // @forinit
	}
}
