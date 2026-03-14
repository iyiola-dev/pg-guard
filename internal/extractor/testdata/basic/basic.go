package basic

import (
	"context"
	"database/sql"
	"fmt"
)

func literalQuery(db *sql.DB) {
	db.Query("SELECT id FROM users WHERE id = $1", 1)
}

func sprintfQuery(db *sql.DB) {
	name := "alice"
	db.Query(fmt.Sprintf("SELECT id FROM users WHERE name = '%s'", name))
}

func concatQuery(db *sql.DB) {
	name := "alice"
	db.Query("SELECT id FROM users WHERE name = '" + name + "'")
}

func contextQuery(db *sql.DB) {
	ctx := context.Background()
	db.QueryContext(ctx, "SELECT id FROM users WHERE id = $1", 1)
}

func loopQuery(db *sql.DB, ids []int) {
	for _, id := range ids {
		db.Query("SELECT name FROM users WHERE id = $1", id)
	}
}
