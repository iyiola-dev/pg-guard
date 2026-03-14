# pg-guard

Postgres Query Linter & Risk Analyzer for Go Code.

A static analysis tool (CLI + `golangci-lint` plugin) that finds risky SQL patterns in Go code and validates them against a live Postgres instance.

## Features

### Static Analysis (no database required)

- **Query extraction** – Parses Go source to find SQL queries in `database/sql`, `pgx`, and GORM calls.
- **Unparameterized query detection** – Flags string-concatenated or `fmt.Sprintf`-based queries vulnerable to SQL injection.
- **Missing context timeouts** – Detects `db.Query()` / `db.Exec()` calls that should use `Context` variants.
- **N+1 loop detection** – Identifies queries executed inside loops.
- **SQL syntax validation** – Parses extracted SQL using the Postgres parser and flags syntax errors.

### Live Database Analysis (optional)

Connects to a running Postgres instance to enrich static findings with real schema data:

- **Full-table scan detection** – Uses `pg_stat_user_tables` row estimates to flag queries on large tables with no supporting index.
- **Schema validation** – Verifies that referenced tables exist in the database.
- **Missing index suggestions** – Runs `EXPLAIN` on extracted queries and recommends indexes.

## Installation

```sh
go install github.com/iyiola-dev/pg-guard/cmd/pg-guard@latest
```

Verify the installation:

```sh
pg-guard --help
```

## Usage

### Quick Start

Run pg-guard against your project — no configuration needed:

```sh
pg-guard ./...
```

### Static Analysis

Static checks work out of the box on any Go project that uses `database/sql`, `pgx`, or GORM:

```sh
# Analyze all packages
pg-guard ./...

# Analyze a specific package
pg-guard ./internal/repository/...
```

Sample output:

```
handlers.go:63:10  error    invalid-sql    invalid SQL syntax: syntax error at or near "name"
handlers.go:98:21  warning  n-plus-one     query executed inside loop body (N+1 pattern)
```

### Live Database Analysis

Pass a `--dsn` flag to enable checks that query a real Postgres instance:

```sh
pg-guard --dsn "$PG_GUARD_DSN" ./...
```

This adds schema validation, full-table scan detection, and missing index suggestions on top of the static checks.

> **⚠️ Never hardcode credentials in scripts or CI configs.** Use an environment variable:
>
> ```sh
> export PG_GUARD_DSN="postgres://..."
> pg-guard --dsn "$PG_GUARD_DSN" ./...
> ```

### JSON Output

```sh
pg-guard --format json ./...
```

```json
[
  {
    "file": "handlers.go",
    "line": 63,
    "column": 10,
    "severity": "error",
    "check": "invalid-sql",
    "message": "invalid SQL syntax: syntax error at or near \"name\"",
    "sql": "name"
  }
]
```

### Exit Codes

| Code | Meaning |
|------|---------|
| `0`  | No issues found |
| `1`  | Findings with `error` severity |
| `2`  | pg-guard itself failed (bad flags, load error, etc.) |

### CI Integration

Add pg-guard to your CI pipeline as a lint step:

```yaml
# GitHub Actions example
- name: Install pg-guard
  run: go install github.com/iyiola-dev/pg-guard/cmd/pg-guard@latest

- name: Run pg-guard
  run: pg-guard ./...
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
```

Set the DSN via environment variable for live checks:

```sh
export PG_GUARD_DSN="postgres://..."
golangci-lint run
```

### Makefile Integration

```makefile
.PHONY: lint lint-live

lint:
	pg-guard ./...

lint-live:
	pg-guard --dsn "$$PG_GUARD_DSN" ./...
```

## Checks

| Check | Severity | Requires DB | Description |
|-------|----------|-------------|-------------|
| `unparameterized-query` | error | no | SQL built with `fmt.Sprintf` or string concatenation |
| `invalid-sql` | error | no | SQL string fails Postgres parser validation |
| `missing-context` | warning | no | `db.Query()` / `db.Exec()` used without `Context` variant |
| `n-plus-one` | warning | no | Query executed inside a loop body |
| `invalid-schema` | error | yes | Referenced table does not exist in the database |
| `missing-index` | info | yes | Query causes full-table scan on a large table |

## Project Structure

```
cmd/
  pg-guard/          CLI entrypoint
internal/
  analyzer/          Core analysis engine
  extractor/         Go AST query extraction (database/sql, pgx, GORM)
  checks/            Individual lint checks
  dbinfo/            Live Postgres introspection (indexes, row counts, EXPLAIN)
  report/            Report formatting (text, JSON)
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
