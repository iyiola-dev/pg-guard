// Package pgxpool is a minimal stand-in for github.com/jackc/pgx/v5/pgxpool.
package pgxpool

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Pool struct{}

func (p *Pool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) { return nil, nil }

func (p *Pool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row { return nil }

func (p *Pool) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (p *Pool) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults { return nil }

func (p *Pool) Begin(ctx context.Context) (pgx.Tx, error) { return nil, nil }
