package analyzer

import (
	"context"
	"fmt"
	"testing"
	"time"

	pgguard "github.com/iyiola-dev/pg-guard"
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

func TestRun_StaticOnly(t *testing.T) {
	cfg := pgguard.Config{
		Patterns: []string{"./testdata/sample"},
		Format:   "text",
	}

	findings, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected findings from static analysis")
	}

	var hasUnparam bool
	for _, f := range findings {
		if f.Check == pgguard.CheckUnparameterizedQuery {
			hasUnparam = true
		}
	}

	if !hasUnparam {
		t.Error("expected unparameterized-query finding")
	}
}

func TestRun_WithDB(t *testing.T) {
	dsn, cleanup := setupPostgres(t)
	defer cleanup()

	cfg := pgguard.Config{
		Patterns: []string{"./testdata/sample"},
		DSN:      dsn,
		Format:   "text",
	}

	findings, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected findings from analysis with DB")
	}
}
