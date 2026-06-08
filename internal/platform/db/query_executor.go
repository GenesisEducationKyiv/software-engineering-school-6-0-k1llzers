package db

import (
	"context"
	"database/sql"
)

type QueryExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func NewQueryExecutor(ctx context.Context, db *sql.DB) QueryExecutor {
	tx := TxFromContext(ctx)
	if tx != nil {
		return tx
	}

	return db
}
