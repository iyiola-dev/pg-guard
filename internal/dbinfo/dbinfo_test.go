package dbinfo

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/iyiola-dev/pg-guard/internal/pgtest"
	"github.com/jackc/pgx/v5/pgconn"
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
	for _, image := range []string{pg16, pg15} {
		t.Run(image, func(t *testing.T) {
			db, _ := open(t, image, plannerSchema...)
			testExplain(t, db)
		})
	}
}

func testExplain(t *testing.T, db *DB) {
	type scan struct {
		filtered, limited bool
	}
	tests := []struct {
		name string
		sql  string
		want map[string]scan // by relation
	}{
		{
			name: "beneath sort",
			sql:  "SELECT id FROM users WHERE name = $1 ORDER BY email",
			want: map[string]scan{"users": {filtered: true}},
		},
		{
			name: "beneath aggregate",
			sql:  "SELECT user_id, sum(amount) FROM orders GROUP BY user_id",
			want: map[string]scan{"orders": {}},
		},
		{
			name: "beneath join",
			sql:  "SELECT o.id FROM orders o JOIN users u ON u.id = o.user_id WHERE u.name = $1",
			want: map[string]scan{"orders": {}, "users": {filtered: true}},
		},
		{
			name: "beneath limit",
			sql:  "SELECT * FROM users LIMIT 5",
			want: map[string]scan{"users": {limited: true}},
		},
		{
			name: "index scan",
			sql:  "SELECT name FROM users WHERE id = $1",
			want: map[string]scan{},
		},
		{
			name: "parameterized insert",
			sql:  "INSERT INTO orders (id, user_id, amount) VALUES ($1, $2, $3)",
			want: map[string]scan{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := db.Explain(context.Background(), tt.sql)
			if err != nil {
				t.Fatalf("Explain: %v", err)
			}
			got := make(map[string]scan)
			for _, s := range result.SeqScans() {
				if s.Schema != "public" {
					t.Errorf("scan of %s: schema = %q, want public", s.Relation, s.Schema)
				}
				got[s.Relation] = scan{filtered: s.Filter != "", limited: s.Limited}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("seq scans = %+v, want %+v", got, tt.want)
			}
			for rel, want := range tt.want {
				if got[rel] != want {
					t.Errorf("scan of %s = %+v, want %+v", rel, got[rel], want)
				}
			}
		})
	}

	// Failures are Postgres errors, and leave the connection usable.
	failing := []string{
		"SELECT id FROM users WHERE no_such_fn($1)", // fails to prepare
		"SELECT id FROM users WHERE id = $1 + 1/0",  // fails while planning
		"SELECT nme FROM users",
	}
	for _, sql := range failing {
		_, err := db.Explain(context.Background(), sql)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) {
			t.Errorf("Explain(%q) = %v, want a Postgres error", sql, err)
		}
		if _, err := db.Explain(context.Background(), "SELECT name FROM users WHERE id = $1"); err != nil {
			t.Fatalf("Explain after %q failed: %v", sql, err)
		}
	}
}

func TestExplain_RejectsOtherStatements(t *testing.T) {
	db, _ := open(t, pg16, usersSchema...)
	ctx := context.Background()

	for _, sql := range []string{
		"SELECT 1; DROP TABLE users",
		"DROP TABLE users",
		"CREATE TABLE copy AS SELECT * FROM users",
		"SELEC 1",
	} {
		if _, err := db.Explain(ctx, sql); err == nil {
			t.Errorf("Explain(%q) succeeded, want an error", sql)
		}
	}
	if exists, err := db.TableExists(ctx, "users"); err != nil || !exists {
		t.Fatalf("users table is gone (exists=%v, err=%v)", exists, err)
	}
}

func TestExplain_DoesNotWrite(t *testing.T) {
	db, _ := open(t, pg16, usersSchema...)
	ctx := context.Background()

	for _, sql := range []string{
		"INSERT INTO users (name, email) VALUES ($1, $2)",
		"UPDATE users SET name = 'changed'",
		"DELETE FROM users",
	} {
		if _, err := db.Explain(ctx, sql); err != nil {
			t.Fatalf("Explain(%q): %v", sql, err)
		}
	}

	var count, changed int
	err := db.conn.QueryRow(ctx, "SELECT count(*), count(*) FILTER (WHERE name = 'changed') FROM users").Scan(&count, &changed)
	if err != nil {
		t.Fatal(err)
	}
	if count != 100 || changed != 0 {
		t.Errorf("users has %d rows, %d changed; EXPLAIN must not modify data", count, changed)
	}
}

func TestExplain_ParameterizedUsesIndex(t *testing.T) {
	db, _ := open(t, pg16,
		`CREATE TABLE orders (id SERIAL PRIMARY KEY, user_id INT NOT NULL, amount INT)`,
		`CREATE INDEX idx_orders_user_id ON orders (user_id)`,
		`INSERT INTO orders (user_id, amount) SELECT i % 1000, i FROM generate_series(1, 50000) AS i`,
		`ANALYZE orders`,
	)

	result, err := db.Explain(context.Background(), "SELECT * FROM orders WHERE user_id = $1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scans := result.SeqScans(); len(scans) != 0 {
		t.Errorf("expected an index scan for an indexed lookup, got seq scans %+v", scans)
	}
	if !strings.Contains(result.Plan.NodeType, "Index") && !strings.Contains(result.Plan.NodeType, "Bitmap") {
		t.Errorf("top plan node = %q, want an index or bitmap scan", result.Plan.NodeType)
	}
}
