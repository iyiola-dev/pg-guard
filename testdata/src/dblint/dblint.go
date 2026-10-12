// Package dblint is checked against a live database seeded by
// plugin/plugin_test.go.
package dblint

import (
	"context"
	"database/sql"

	"gorm.io/gorm"
)

type User struct {
	ID int
}

func queries(ctx context.Context, db *sql.DB) {
	db.QueryContext(ctx, "SELECT id FROM missing_table") // want `\[invalid-schema\] table missing_table does not exist`
	db.QueryContext(ctx, "SELECT nme FROM users")        // want `\[invalid-schema\] column "nme" does not exist`

	// Two queries on the same table are both planned.
	db.QueryContext(ctx, "SELECT id FROM users WHERE email = $1 ORDER BY name", "a")            // want `\[missing-index\] sequential scan on public.users \(~150k rows\) filtering on email; consider an index`
	db.QueryContext(ctx, "SELECT name, count(*) FROM users WHERE city = $1 GROUP BY name", "x") // want `\[missing-index\] .* filtering on city`
	db.QueryContext(ctx, "SELECT id FROM users WHERE id = $1", 1)
	db.QueryContext(ctx, "SELECT id FROM users LIMIT 10")

	db.QueryContext(ctx, "SELECT o.id FROM orders o JOIN users u ON u.id = o.user_id WHERE u.id = $1", 1) // want `\[missing-index\] sequential scan on public.orders .* filtering on user_id`
	db.QueryContext(ctx, "SELECT user_id, sum(amount) FROM orders GROUP BY user_id")                      // want `\[full-table-scan\] query reads all of public.orders \(~\d+k rows\) with a sequential scan`
	db.QueryContext(ctx, "SELECT id FROM users WHERE no_such_fn($1)", 1)                                  // want `\[db-analysis-failed\] could not plan query: function no_such_fn\(unknown\) does not exist`

	db.ExecContext(ctx, "CREATE TEMP TABLE scratch (id int)")
	db.ExecContext(ctx, "INSERT INTO scratch (id) VALUES ($1)", 1)
}

func gormQueries(ctx context.Context, db *gorm.DB) {
	var users []User
	db.WithContext(ctx).Raw("SELECT * FROM users WHERE name = ?", "a").Scan(&users) // want `\[missing-index\] .* filtering on name`
	db.WithContext(ctx).Raw("SELECT * FROM users WHERE id IN ?", []int{1}).Scan(&users)
	db.WithContext(ctx).Exec("UPDATE users SET name = ? WHERE id = ?", "b", 1)
}
