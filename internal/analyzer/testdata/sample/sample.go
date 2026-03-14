package sample

import (
	"database/sql"
	"fmt"
)

func unsafeQuery(db *sql.DB) {
	name := "alice"
	db.Query(fmt.Sprintf("SELECT id FROM users WHERE name = '%s'", name))
}

func noContextQuery(db *sql.DB) {
	db.Query("SELECT id FROM users WHERE id = $1", 1)
}
