// Package pgx is a minimal stand-in for github.com/jackc/pgx/v5 with the
// same method signatures.
package pgx

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
)

type Rows interface {
	Close()
	Next() bool
	Scan(dest ...any) error
}

type Row interface {
	Scan(dest ...any) error
}

type NamedArgs map[string]any

type StrictNamedArgs map[string]any

type QueuedQuery struct{}

type Batch struct{}

func (b *Batch) Queue(query string, arguments ...any) *QueuedQuery { return &QueuedQuery{} }

func (b *Batch) Len() int { return 0 }

type BatchResults interface {
	Close() error
}

type Conn struct{}

func (c *Conn) Query(ctx context.Context, sql string, args ...any) (Rows, error) { return nil, nil }

func (c *Conn) QueryRow(ctx context.Context, sql string, args ...any) Row { return nil }

func (c *Conn) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (c *Conn) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (c *Conn) SendBatch(ctx context.Context, b *Batch) BatchResults { return nil }

func (c *Conn) Begin(ctx context.Context) (Tx, error) { return nil, nil }

func (c *Conn) Ping(ctx context.Context) error { return nil }

type Tx interface {
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	SendBatch(ctx context.Context, b *Batch) BatchResults
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}
