package dbinfo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupPostgres(t *testing.T) (string, func()) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	dsn := fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())

	cleanup := func() {
		_ = container.Terminate(ctx)
	}

	return dsn, cleanup
}

func seedSchema(t *testing.T, dsn string) {
	t.Helper()
	ctx := context.Background()

	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer db.Close(ctx)

	queries := []string{
		`CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)`,
		`CREATE INDEX idx_users_email ON users (email)`,
		`INSERT INTO users (name, email) SELECT 'user' || i, 'user' || i || '@test.com' FROM generate_series(1, 100) AS i`,
		`ANALYZE users`,
	}
	for _, q := range queries {
		_, err := db.conn.Exec(ctx, q)
		if err != nil {
			t.Fatalf("seed query failed: %v\nquery: %s", err, q)
		}
	}
}

func TestTableExists(t *testing.T) {
	dsn, cleanup := setupPostgres(t)
	defer cleanup()
	seedSchema(t, dsn)

	ctx := context.Background()
	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer db.Close(ctx)

	exists, err := db.TableExists(ctx, "users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected users table to exist")
	}

	exists, err = db.TableExists(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected nonexistent table to not exist")
	}
}

func TestColumnExists(t *testing.T) {
	dsn, cleanup := setupPostgres(t)
	defer cleanup()
	seedSchema(t, dsn)

	ctx := context.Background()
	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer db.Close(ctx)

	exists, err := db.ColumnExists(ctx, "users", "email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected email column to exist")
	}

	exists, err = db.ColumnExists(ctx, "users", "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected nonexistent column to not exist")
	}
}

func TestTableRowEstimate(t *testing.T) {
	dsn, cleanup := setupPostgres(t)
	defer cleanup()
	seedSchema(t, dsn)

	ctx := context.Background()
	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer db.Close(ctx)

	estimate, err := db.TableRowEstimate(ctx, "users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if estimate <= 0 {
		t.Errorf("expected positive row estimate, got %d", estimate)
	}
}

func TestTableIndexes(t *testing.T) {
	dsn, cleanup := setupPostgres(t)
	defer cleanup()
	seedSchema(t, dsn)

	ctx := context.Background()
	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer db.Close(ctx)

	indexes, err := db.TableIndexes(ctx, "users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(indexes) == 0 {
		t.Fatal("expected at least one index on users")
	}

	var foundEmail bool
	for _, idx := range indexes {
		if idx.Name == "idx_users_email" {
			foundEmail = true
			if len(idx.Columns) != 1 || idx.Columns[0] != "email" {
				t.Errorf("expected email column in index, got %v", idx.Columns)
			}
		}
	}
	if !foundEmail {
		t.Error("expected idx_users_email index")
	}
}

func TestExplain(t *testing.T) {
	dsn, cleanup := setupPostgres(t)
	defer cleanup()
	seedSchema(t, dsn)

	ctx := context.Background()
	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer db.Close(ctx)

	result, err := db.Explain(ctx, "SELECT * FROM users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TableName != "users" {
		t.Errorf("expected table name 'users', got %q", result.TableName)
	}
	// With 100 rows and no WHERE clause, should be a seq scan
	if !result.SeqScan {
		t.Error("expected seq scan for full table select")
	}
}

func TestExplain_IndexScan(t *testing.T) {
	dsn, cleanup := setupPostgres(t)
	defer cleanup()

	ctx := context.Background()
	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer db.Close(ctx)

	// Insert enough rows so Postgres prefers an index scan over seq scan
	stmts := []string{
		`CREATE TABLE orders (id SERIAL PRIMARY KEY, user_id INT NOT NULL, amount INT)`,
		`CREATE INDEX idx_orders_user_id ON orders (user_id)`,
		`INSERT INTO orders (user_id, amount) SELECT i % 1000, i FROM generate_series(1, 50000) AS i`,
		`ANALYZE orders`,
	}
	for _, s := range stmts {
		if _, err := db.conn.Exec(ctx, s); err != nil {
			t.Fatalf("setup failed: %v\nquery: %s", err, s)
		}
	}

	result, err := db.Explain(ctx, "SELECT * FROM orders WHERE user_id = 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SeqScan {
		t.Error("expected index scan for indexed column lookup on large table, got seq scan")
	}
}
