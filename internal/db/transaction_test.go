//go:build integration

package db

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransactionManager_WithinTransaction_Commits(t *testing.T) {
	_, db := setupTestPostgres(t)
	manager := NewTransactionManager(db)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `create table tx_test (value integer not null)`)
	require.NoError(t, err)

	err = manager.WithinTransaction(ctx, func(ctx context.Context) error {
		tx := TxFromContext(ctx)
		_, execErr := tx.ExecContext(ctx, `insert into tx_test (value) values (1)`)
		return execErr
	})
	require.NoError(t, err)

	var count int
	err = db.QueryRowContext(ctx, `select count(*) from tx_test`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestTransactionManager_WithinTransaction_RollsBackOnError(t *testing.T) {
	_, db := setupTestPostgres(t)
	manager := NewTransactionManager(db)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `create table tx_test (value integer not null)`)
	require.NoError(t, err)

	expectedErr := errors.New("boom")
	err = manager.WithinTransaction(ctx, func(ctx context.Context) error {
		tx := TxFromContext(ctx)
		_, execErr := tx.ExecContext(ctx, `insert into tx_test (value) values (1)`)
		require.NoError(t, execErr)
		return expectedErr
	})
	require.ErrorIs(t, err, expectedErr)

	var count int
	err = db.QueryRowContext(ctx, `select count(*) from tx_test`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestTransactionManager_WithinTransaction_RollsBackOnPanic(t *testing.T) {
	_, db := setupTestPostgres(t)
	manager := NewTransactionManager(db)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `create table tx_test (value integer not null)`)
	require.NoError(t, err)

	require.PanicsWithValue(t, "panic in transaction", func() {
		_ = manager.WithinTransaction(ctx, func(ctx context.Context) error {
			tx := TxFromContext(ctx)
			_, execErr := tx.ExecContext(ctx, `insert into tx_test (value) values (1)`)
			require.NoError(t, execErr)
			panic("panic in transaction")
		})
	})

	var count int
	err = db.QueryRowContext(ctx, `select count(*) from tx_test`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}
