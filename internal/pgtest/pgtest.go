// Package pgtest runs throwaway Postgres servers in Docker for integration
// tests.
package pgtest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	mu      sync.Mutex
	servers = make(map[string]*server)
	serial  atomic.Int64
)

type server struct {
	container testcontainers.Container
	host      string
	port      string
	err       error
}

func (s *server) dsn(db string) string {
	return fmt.Sprintf("postgres://test:test@%s:%s/%s?sslmode=disable", s.host, s.port, db)
}

// Database creates an empty database on a server running image, starting
// the server on first use, and returns the database's connection string.
// The database is dropped when the test ends. In -short mode the test is
// skipped instead.
func Database(t testing.TB, image string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Postgres integration test in -short mode")
	}

	s := start(image)
	if s.err != nil {
		t.Fatalf("pgtest: start %s: %v", image, s.err)
	}
	name := fmt.Sprintf("pgguard_%d", serial.Add(1))
	Exec(t, s.dsn("postgres"), "CREATE DATABASE "+name)
	t.Cleanup(func() {
		ctx := context.Background()
		if conn, err := pgx.Connect(ctx, s.dsn("postgres")); err == nil {
			_, _ = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			_ = conn.Close(ctx)
		}
	})
	return s.dsn(name)
}

// Exec runs statements against dsn, failing the test on error.
func Exec(t testing.TB, dsn string, statements ...string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("pgtest: connect: %v", err)
	}
	defer conn.Close(ctx)
	for _, stmt := range statements {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("pgtest: %v\nstatement: %s", err, stmt)
		}
	}
}

// Stop removes the servers started by Database. Call it from TestMain.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	for image, s := range servers {
		if s.container != nil {
			_ = s.container.Terminate(context.Background())
		}
		delete(servers, image)
	}
}

func start(image string) *server {
	mu.Lock()
	defer mu.Unlock()
	if s, ok := servers[image]; ok {
		return s
	}
	s := &server{}
	servers[image] = s

	ctx := context.Background()
	s.container, s.err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER":     "test",
				"POSTGRES_PASSWORD": "test",
			},
			// The entrypoint restarts the server after initdb, so the
			// second ready message is the one that counts.
			WaitingFor: wait.ForAll(
				wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
				wait.ForListeningPort("5432/tcp"),
			).WithDeadline(2 * time.Minute),
		},
		Started: true,
	})
	if s.err != nil {
		return s
	}
	if s.host, s.err = s.container.Host(ctx); s.err != nil {
		return s
	}
	port, err := s.container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		s.err = err
		return s
	}
	s.port = port.Port()
	return s
}
