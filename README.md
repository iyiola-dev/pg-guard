# pg-guard

Postgres Query Linter & Risk Analyzer for Go Code.

A static analysis tool (CLI + `golangci-lint` plugin) that finds risky SQL patterns in Go code and validates them against a live Postgres instance.

## Features

### Static Analysis (no database required)

- **Query extraction** – Finds SQL passed to `database/sql`, `pgx` and GORM, recognizing calls by the type that declares the method rather than by name alone. See [Supported Code](#supported-code).
- **Unparameterized query detection** – Flags SQL built with `fmt.Sprintf` or string concatenation from non-constant values. SQL assembled only from constants is not flagged.
- **Missing context timeouts** – Detects `db.Query()` / `db.Exec()` calls that should use `Context` variants, and GORM queries that run without `WithContext(ctx)`.
- **N+1 loop detection** – Identifies queries executed inside loops.
- **SQL syntax validation** – Parses extracted SQL, including GORM condition fragments, with the Postgres parser and flags syntax errors.

### Live Database Analysis (optional)

Connects to a running Postgres instance to enrich static findings with real schema data:

- **Schema validation** – Verifies that the tables and columns a query references exist.
- **Missing index suggestions** – Plans each query with `EXPLAIN` and reports sequential scans that filter a large table, naming the filtered columns. Scans are found anywhere in the plan, including beneath sorts, aggregates and joins.
- **Full-table scan detection** – Reports queries that read every row of a large table.

## Installation

pg-guard requires Go 1.25+ and cgo with a C compiler, because it embeds the Postgres parser ([pg_query_go](https://github.com/pganalyze/pg_query_go)). The first build compiles the parser and takes a minute or two.

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

# Include _test.go files
pg-guard --tests ./...
```

Sample output:

```
handlers.go:10:5  error    unparameterized-query      SQL built with fmt.Sprintf with non-constant arguments; use a parameterized query instead
handlers.go:11:5  error    invalid-sql                invalid SQL syntax: syntax error at end of input
handlers.go:12:5  warning  missing-context            Query() called without context; use QueryContext()
handlers.go:14:6  warning  n-plus-one                 query executed inside loop body (N+1 pattern)
```

### Live Database Analysis

Pass a `--dsn` flag to enable checks that query a real Postgres instance:

```sh
pg-guard --dsn "$PG_GUARD_DSN" ./...
```

This adds schema validation, missing index suggestions and full-table scan detection on top of the static checks:

```
handlers.go:16:5  info     missing-index              sequential scan on public.users (~200k rows) filtering on email; consider an index
```

pg-guard never runs your queries. Each query is planned with `EXPLAIN` (without `ANALYZE`) inside a read-only transaction that is rolled back. Queries with `$1`-style placeholders are planned generically: with `EXPLAIN (GENERIC_PLAN)` on Postgres 16+, and through a prepared statement with `plan_cache_mode = force_generic_plan` on older servers.

Index and scan findings only cover tables with at least 100,000 estimated rows. If Postgres rejects a query while planning it, pg-guard reports `db-analysis-failed` with the reason, or `invalid-schema` when the error is a missing table, column, type or schema. Tables the analyzed code creates itself, such as temporary tables, are not checked against the database.

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
    "line": 10,
    "column": 5,
    "severity": "error",
    "check": "unparameterized-query",
    "message": "SQL built with fmt.Sprintf with non-constant arguments; use a parameterized query instead",
    "sql": "SELECT id FROM users WHERE name = '%s'",
    "suggested_fix": "Use $1, $2, ... placeholders and pass values as arguments"
  }
]
```

### Exit Codes

| Code | Meaning |
|------|---------|
| `0`  | No `error` findings (warnings and info findings may still be printed) |
| `1`  | Findings with `error` severity |
| `2`  | pg-guard itself failed (bad flags, load error, database unreachable, etc.) |

### CI Integration

Add pg-guard to your CI pipeline as a lint step. GitHub's Ubuntu runners include the C compiler pg-guard needs; on Alpine, install `build-base` first.

```yaml
# GitHub Actions example
- uses: actions/setup-go@v5
  with:
    go-version: "1.25"

- name: Install pg-guard
  run: go install github.com/iyiola-dev/pg-guard/cmd/pg-guard@latest

- name: Run pg-guard
  run: pg-guard ./...
```

### golangci-lint Plugin

pg-guard is a [module plugin](https://golangci-lint.run/docs/plugins/module-plugins/) for golangci-lint v2. Module plugins are compiled into a custom golangci-lint binary.

1. Describe the custom binary in `.custom-gcl.yml`:

   ```yaml
   version: v2.9.0 # the golangci-lint version to build
   plugins:
     - module: github.com/iyiola-dev/pg-guard
       import: github.com/iyiola-dev/pg-guard/plugin
       version: latest # or a tag or commit
   ```

2. Build it. This writes `./custom-gcl`, and needs the same C compiler as `go install`:

   ```sh
   golangci-lint custom
   ```

3. Enable the linter in `.golangci.yml`:

   ```yaml
   version: "2"
   linters:
     enable:
       - pgguard
     settings:
       custom:
         pgguard:
           type: module
           description: Postgres query linter and risk analyzer
   ```

4. Run the custom binary instead of `golangci-lint`:

   ```sh
   ./custom-gcl run ./...
   ```

The plugin analyzes the files golangci-lint gives it, including test files unless `run.tests` is `false`. Set `PG_GUARD_DSN` to enable the live database checks:

```sh
export PG_GUARD_DSN="postgres://..."
./custom-gcl run ./...
```

Each diagnostic starts with the check name, e.g. `[missing-context]`, which `linters.exclusions.rules` can match. By default golangci-lint shows one issue per line (`issues.uniq-by-line`), so a line with several findings shows only one of them.

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
| `unparameterized-query` | error | no | SQL built with `fmt.Sprintf` or string concatenation from non-constant values |
| `invalid-sql` | error | no | SQL string or GORM fragment fails Postgres parser validation |
| `missing-context` | warning | no | `db.Query()` / `db.Exec()` used without `Context` variant, or a GORM query without `WithContext` |
| `n-plus-one` | warning | no | Query executed inside a loop body |
| `invalid-schema` | error | yes | Referenced table, column, type or schema does not exist in the database |
| `missing-index` | info | yes | Sequential scan filtering a large table; names the filtered columns |
| `full-table-scan` | info | yes | Query reads every row of a large table |
| `db-analysis-failed` | warning | yes | Postgres could not plan the query; the message gives its reason |

## Supported Code

| Library | Recognized calls |
|---------|------------------|
| `database/sql` | `Query`, `QueryRow`, `Exec`, `Prepare` and their `Context` variants on `*sql.DB`, `*sql.Tx` and `*sql.Conn`, including types that embed them; executions of a prepared `*sql.Stmt` |
| `pgx` v4 and v5 | `Query`, `QueryRow`, `Exec` and `Prepare` on `*pgx.Conn`, `pgx.Tx`, `*pgxpool.Pool`, `*pgxpool.Conn` and `*pgxpool.Tx`; `(*pgx.Batch).Queue` |
| GORM | `Raw` and `Exec`; condition fragments in `Where`, `Or`, `Not`, `Having` and inline conditions such as `First(&u, "name = ?", name)`; `Select`, `Order`, `Group`, `Joins`, `Table`, `Pluck` and `gorm.Expr` |
| Interfaces and wrappers | Methods shaped like the ones above that return `database/sql` or pgx result types, such as the `DBTX` interfaces sqlc generates |

The SQL argument can be a literal, a constant (including constants built by concatenation), a local or package-level variable assigned in the same package, or `fmt.Sprintf` over constants. GORM `?` and `@name` placeholders and `pgx.NamedArgs` are rewritten to `$N` before parsing and planning, with `IN ?` expanded the way GORM does.

GORM queries are checked for a context only when the chain plainly lacks one, e.g. it starts from a struct field or package variable without `WithContext`. A chain that starts from a `*gorm.DB` parameter, such as the `tx` in a `Transaction` callback, is assumed to carry its caller's context.

Not analyzed: SQL returned by functions or read from struct fields, sqlx-specific methods (`Select`, `Get`, `NamedExec`, ...), and GORM's generics API (`gorm.G[T]`).

## Project Structure

```
cmd/
  pg-guard/          CLI entrypoint
internal/
  analyzer/          Core analysis engine and live database checks
  extractor/         Go AST query extraction (database/sql, pgx, GORM)
  checks/            Static lint checks
  sqltext/           Placeholder rewriting and Postgres parser helpers
  dbinfo/            Live Postgres introspection (tables, row estimates, EXPLAIN)
  report/            Report formatting (text, JSON)
  pgtest/            Postgres containers for integration tests
plugin/              golangci-lint plugin and go/analysis analyzer
testdata/            Analyzer test cases and stub pgx and GORM packages
```

## Requirements

- Go 1.25+ with cgo and a C compiler
- Docker (for running integration tests)
- Postgres 12+ for live analysis (the integration tests run against 15 and 16)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to fork, branch, and open a pull request.

## License

[MIT](LICENSE)
