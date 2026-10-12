// Package pgconn is a minimal stand-in for github.com/jackc/pgx/v5/pgconn.
package pgconn

type CommandTag struct {
	s string
}

type StatementDescription struct {
	Name string
	SQL  string
}
