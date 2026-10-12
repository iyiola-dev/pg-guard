package dbinfo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/iyiola-dev/pg-guard/internal/pgtest"
)

const (
	pg16 = "postgres:16-alpine"
	pg15 = "postgres:15-alpine"
)

func TestMain(m *testing.M) {
	code := m.Run()
	pgtest.Stop()
	os.Exit(code)
}

var usersSchema = []string{
	`CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL)`,
	`CREATE INDEX idx_users_email ON users (email)`,
	`INSERT INTO users (name, email) SELECT 'user' || i, 'user' || i || '@test.com' FROM generate_series(1, 100) AS i`,
	`ANALYZE users`,
}

var plannerSchema = []string{
	`CREATE TABLE users (id int PRIMARY KEY, name text NOT NULL, email text NOT NULL)`,
	`CREATE TABLE orders (id int PRIMARY KEY, user_id int NOT NULL, amount int NOT NULL)`,
	`INSERT INTO users SELECT i, 'user' || i, 'user' || i || '@test.com' FROM generate_series(1, 10000) AS i`,
	`INSERT INTO orders SELECT i, i % 10000, i FROM generate_series(1, 20000) AS i`,
	`ANALYZE`,
}

// open connects to a new database on a server running image, seeded with
// statements.
func open(t *testing.T, image string, statements ...string) (*DB, string) {
	t.Helper()
	dsn := pgtest.Database(t, image)
	pgtest.Exec(t, dsn, statements...)

	ctx := context.Background()
	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(func() { db.Close(ctx) })
	return db, dsn
}

func TestTableExists(t *testing.T) {
	db, _ := open(t, pg16, append(usersSchema,
		`CREATE SCHEMA app`,
		`CREATE TABLE app."MixedCase" (id int)`,
	)...)

	cases := map[string]bool{
		"users":           true,
		"public.users":    true,
		`"users"`:         true,
		"nonexistent":     false,
		`app."MixedCase"`: true,
		"app.mixedcase":   false,
		`"MixedCase"`:     false, // app is not on the search path
	}
	for table, want := range cases {
		exists, err := db.TableExists(context.Background(), table)
		if err != nil {
			t.Fatalf("TableExists(%s): %v", table, err)
		}
		if exists != want {
			t.Errorf("TableExists(%s) = %v, want %v", table, exists, want)
		}
	}
}

func TestColumnExists(t *testing.T) {
	db, _ := open(t, pg16, usersSchema...)
	ctx := context.Background()

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
	db, _ := open(t, pg16, append(usersSchema,
		`CREATE TABLE events (id int)`,
		`INSERT INTO events SELECT generate_series(1, 5000)`,
		`ANALYZE events`,
	)...)
	ctx := context.Background()

	estimate, err := db.TableRowEstimate(ctx, "users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if estimate != 100 {
		t.Errorf("users estimate = %d, want 100", estimate)
	}

	// Once the seeding session's statistics are flushed, n_live_tup counts
	// the inserted rows a second time on top of ANALYZE's count. The
	// estimate must not follow it.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var live int64
		if err := db.conn.QueryRow(ctx, "SELECT n_live_tup FROM pg_stat_user_tables WHERE relname = 'events'").Scan(&live); err != nil {
			t.Fatal(err)
		}
		if live > 5000 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	estimate, err = db.TableRowEstimate(ctx, "events")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if estimate != 5000 {
		t.Errorf("events estimate = %d, want 5000", estimate)
	}

	if _, err := db.TableRowEstimate(ctx, "nonexistent"); err == nil {
		t.Error("expected an error for a missing table")
	}
}

func TestTableIndexes(t *testing.T) {
	db, _ := open(t, pg16, usersSchema...)

	indexes, err := db.TableIndexes(context.Background(), "users")
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
	db, _ := open(t, pg16, usersSchema...)
	result, err := db.Explain(context.Background(), "SELECT * FROM users")
	if err != nil {
		t.Fatal(err)
	}
	scans := result.SeqScans()
	if len(scans) != 1 || scans[0].Relation != "users" {
		t.Fatalf("unexpected scans: %+v", scans)
	}
}
