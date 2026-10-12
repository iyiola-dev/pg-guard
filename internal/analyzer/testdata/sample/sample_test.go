package sample

import (
	"context"
	"database/sql"
	"testing"
)

func TestSample(t *testing.T) {
	var db *sql.DB
	db.QueryContext(context.Background(), "SELEC 1")
}
