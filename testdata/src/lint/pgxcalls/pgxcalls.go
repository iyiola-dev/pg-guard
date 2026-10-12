package pgxcalls

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func conn(ctx context.Context, c *pgx.Conn) {
	c.Query(ctx, "SELEC id FROM users WHERE id = $1", 1) // want `\[invalid-sql\] invalid SQL syntax: syntax error at or near "SELEC"`
	c.QueryRow(ctx, "SELECT id FROM users WHERE id = $1", 1)
	c.Exec(ctx, "DELETE FROM users WHERE id = $1", 1)
	c.Prepare(ctx, "get_user", "SELECT name FROM users WHERE id = $1")
	c.Prepare(ctx, "broken", "SELECT name FROM") // want `\[invalid-sql\] invalid SQL syntax: syntax error at end of input`
	c.Ping(ctx)
}

func pool(ctx context.Context, p *pgxpool.Pool, ids []int, name string) {
	for _, id := range ids {
		p.QueryRow(ctx, "SELECT name FROM users WHERE id = $1", id) // want `\[n-plus-one\] query executed inside loop body`
	}
	p.Exec(ctx, "UPDATE users SET name = '"+name+"'") // want `\[unparameterized-query\] SQL built with string concatenation`
}

func tx(ctx context.Context, t pgx.Tx) {
	t.Exec(ctx, "INSERT INTO users (name) VALUES ($1)", "a")
	t.Query(ctx, "SELECT FROM WHERE") // want `\[invalid-sql\]`
}

func batch(ctx context.Context, c *pgx.Conn, ids []int) {
	b := &pgx.Batch{}
	for _, id := range ids {
		b.Queue("SELECT name FROM users WHERE id = $1", id)
	}
	b.Queue("SELEC 1") // want `\[invalid-sql\]`
	c.SendBatch(ctx, b)
}

func named(ctx context.Context, c *pgx.Conn) {
	c.Query(ctx, "SELECT id FROM users WHERE name = @name AND email = @email", pgx.NamedArgs{"name": "a", "email": "b"})
}
