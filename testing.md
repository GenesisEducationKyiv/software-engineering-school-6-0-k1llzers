# Testing

Repository tests are split into two groups:

- `unit` tests: fast tests that do not require Docker.
- `integration` tests: tests that use PostgreSQL via testcontainers and start their dependencies from scratch in Docker.

Prerequisites:

- `git`
- `docker`
- Go toolchain

## Run unit tests

```bash
go test -tags=unit ./...
```

## Run integration tests

```bash
go test -tags=integration ./...
```

Integration tests automatically create the required Docker containers and apply database migrations. No manual database setup is needed.

## Run all tests

```bash
go test -tags=unit ./... && go test -tags=integration ./...
```

## Add new tests

Every new `_test.go` file must start with one of these build tags:

```go
//go:build unit
```

or

```go
//go:build integration
```

Unit tests belong to the `unit` stage. Tests that need Docker, PostgreSQL, or other external infrastructure belong to the `integration` stage.
