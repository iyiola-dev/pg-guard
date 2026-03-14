# pg-guard

Postgres Query Linter & Risk Analyzer for Go Code.

A static analysis tool (CLI + `golangci-lint` plugin) that finds risky SQL patterns in Go code and validates them against a live Postgres instance.

## Features

### Static Analysis

- **Query extraction** – Parses Go source to find SQL queries in `database/sql`, `pgx`, and GORM calls.
- **Unparameterized query detection** – Flags string-concatenated or `fmt.Sprintf`-based queries vulnerable to SQL injection.
- **Missing context timeouts** – Detects `db.Query()` / `db.Exec()` calls that should use `Context` variants.
- **N+1 loop detection** – Identifies queries executed inside loops.
- **SQL syntax validation** – Parses extracted SQL using the Postgres parser and flags syntax errors.

### Live Database Analysis

Connects to a running Postgres instance to enrich static findings with real schema data:

- **Full-table scan detection** – Uses `pg_stat_user_tables` row estimates to flag queries on large tables with no supporting index.
- **Schema validation** – Verifies that referenced tables exist in the database.
- **Missing index suggestions** – Runs `EXPLAIN` on extracted queries and recommends indexes.
- **Query rewrite suggestions** – Proposes optimized alternatives where possible.

## Installation

```sh
go install github.com/iyiola-dev/pg-guard/cmd/pg-guard@latest
```

## Usage

### CLI

```sh
# Analyze the current package
pg-guard ./...

# Analyze with live database connection
pg-guard --dsn "postgres://user:pass@localhost:5432/mydb" ./...

# Output as JSON
pg-guard --format json ./...
```

### golangci-lint Plugin

Add to `.golangci.yml`:

```yaml
linters:
  enable:
    - pg-guard

linters-settings:
  custom:
    pg-guard:
      path: pg-guard.so
      description: Postgres query linter and risk analyzer
      settings:
        dsn_env: "PG_GUARD_DSN" # reads DSN from this env var (optional)
```

> **⚠️ Never hardcode database credentials in config files.** Use an environment variable:
>
> ```sh
> export PG_GUARD_DSN="postgres://user:pass@localhost:5432/mydb"
> ```

## Project Structure

```
cmd/
  pg-guard/          CLI entrypoint
internal/
  analyzer/          Core analysis engine
  extractor/         Go AST query extraction (database/sql, pgx, GORM)
  checks/            Individual lint checks
  dbinfo/            Live Postgres introspection (indexes, row counts, EXPLAIN)
  report/            Report formatting (text, JSON, SARIF)
plugin/              golangci-lint plugin adapter
```

## Requirements

- Go 1.24+
- Docker (for running integration tests)
- Postgres 13+ (for live analysis features)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to fork, branch, and open a pull request.

## License

[MIT](LICENSE)
