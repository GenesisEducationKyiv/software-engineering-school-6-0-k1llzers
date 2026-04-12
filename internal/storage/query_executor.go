package storage

import (
	"context"
	"database/sql"
)

type queryExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func newQueryExecutor(db *sql.DB, tx *sql.Tx) queryExecutor {
	if tx != nil {
		return tx
	}

	return db
}
