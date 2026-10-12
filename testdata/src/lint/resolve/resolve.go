package resolve

import (
	"context"
	"database/sql"
	"fmt"
)

var listUsers = "SELECT id, name FROM users ORDER BY name"

var brokenList = "SELECT id, name FROM users ORDER"

func assigned(ctx context.Context, db *sql.DB) {
	query := "SELEC id FROM users"
	db.QueryContext(ctx, query) // want `\[invalid-sql\] invalid SQL syntax: syntax error at or near "SELEC"`

	var valid = "SELECT id FROM users WHERE id = $1"
	db.QueryContext(ctx, valid, 1)

	db.QueryContext(ctx, listUsers)
	db.QueryContext(ctx, brokenList) // want `\[invalid-sql\]`
}

func assignedInjection(ctx context.Context, db *sql.DB, name string) {
	query := fmt.Sprintf("SELECT id FROM users WHERE name = '%s'", name)
	db.QueryContext(ctx, query) // want `\[unparameterized-query\] SQL built with fmt.Sprintf`

	where := "name = '" + name + "'"
	q2 := "SELECT id FROM users WHERE " + where
	db.QueryContext(ctx, q2) // want `\[unparameterized-query\] SQL built with string concatenation`
}

func sprintfConstants(ctx context.Context, db *sql.DB) {
	table := "users"
	db.QueryContext(ctx, fmt.Sprintf("SELECT id FROM %s WHERE id = $1", table), 1)
	db.QueryContext(ctx, fmt.Sprintf("SELECT id FROM %s LIMIT %d", "users", 10))
	db.QueryContext(ctx, fmt.Sprintf("SELECT id FROM %s WHERE", table)) // want `\[invalid-sql\]`
}

func branches(ctx context.Context, db *sql.DB, byName bool, name string) {
	query := "SELECT id FROM users"
	if byName {
		query = "SELECT id FROM users WHERE name = $1"
	}
	db.QueryContext(ctx, query, name)

	filtered := "SELECT id FROM users WHERE true"
	if name != "" {
		filtered += " AND name = '" + name + "'"
	}
	db.QueryContext(ctx, filtered) // want `\[unparameterized-query\] SQL built with string concatenation`

	appended := "SELECT id FROM users WHERE name = "
	appended += name
	db.QueryContext(ctx, appended) // want `\[unparameterized-query\] SQL built with string concatenation`

	safe := "SELECT id FROM users WHERE true"
	if byName {
		safe += " AND name = $1"
	}
	db.QueryContext(ctx, safe, name)
}

func unknownOrigin(ctx context.Context, db *sql.DB, query string, build func() string) {
	db.QueryContext(ctx, query)
	db.QueryContext(ctx, build())

	addressed := "SELEC 1"
	update(&addressed)
	db.QueryContext(ctx, addressed)
}

func update(s *string) { *s = "SELECT 1" }
