package dbsql

import (
	"context"
	"database/sql"
	"testing"
)

func TestQueries(t *testing.T) {
	var db *sql.DB
	db.QueryContext(context.Background(), "SELECT * FORM users") // want `\[invalid-sql\]`
}
