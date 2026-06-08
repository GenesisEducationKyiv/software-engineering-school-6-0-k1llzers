//go:build integration

package db_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	appdb "github-release-notifier/internal/platform/db"
	"github-release-notifier/internal/platform/db/test"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTransactionManager_WithinTransaction_Commits(t *testing.T) {
	_, db := test.SetupTestPostgres(t)
	manager := appdb.NewTransactionManager(db)
	ctx := context.Background()
	tableName := uniqueTransactionTestTableName()

	_, err := db.ExecContext(ctx, fmt.Sprintf(`create table %s (value integer not null)`, tableName))
	require.NoError(t, err)

	err = manager.WithinTransaction(ctx, func(ctx context.Context) error {
		tx := appdb.TxFromContext(ctx)
		_, execErr := tx.ExecContext(ctx, fmt.Sprintf(`insert into %s (value) values (1)`, tableName))
		return execErr
	})
	require.NoError(t, err)

	var count int
	err = db.QueryRowContext(ctx, fmt.Sprintf(`select count(*) from %s`, tableName)).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestTransactionManager_WithinTransaction_RollsBackOnError(t *testing.T) {
	_, db := test.SetupTestPostgres(t)
	manager := appdb.NewTransactionManager(db)
	ctx := context.Background()
	tableName := uniqueTransactionTestTableName()

	_, err := db.ExecContext(ctx, fmt.Sprintf(`create table %s (value integer not null)`, tableName))
	require.NoError(t, err)

	expectedErr := errors.New("boom")
	err = manager.WithinTransaction(ctx, func(ctx context.Context) error {
		tx := appdb.TxFromContext(ctx)
		_, execErr := tx.ExecContext(ctx, fmt.Sprintf(`insert into %s (value) values (1)`, tableName))
		require.NoError(t, execErr)
		return expectedErr
	})
	require.ErrorIs(t, err, expectedErr)

	var count int
	err = db.QueryRowContext(ctx, fmt.Sprintf(`select count(*) from %s`, tableName)).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestTransactionManager_WithinTransaction_RollsBackOnPanic(t *testing.T) {
	_, db := test.SetupTestPostgres(t)
	manager := appdb.NewTransactionManager(db)
	ctx := context.Background()
	tableName := uniqueTransactionTestTableName()

	_, err := db.ExecContext(ctx, fmt.Sprintf(`create table %s (value integer not null)`, tableName))
	require.NoError(t, err)

	require.PanicsWithValue(t, "panic in transaction", func() {
		_ = manager.WithinTransaction(ctx, func(ctx context.Context) error {
			tx := appdb.TxFromContext(ctx)
			_, execErr := tx.ExecContext(ctx, fmt.Sprintf(`insert into %s (value) values (1)`, tableName))
			require.NoError(t, execErr)
			panic("panic in transaction")
		})
	})

	var count int
	err = db.QueryRowContext(ctx, fmt.Sprintf(`select count(*) from %s`, tableName)).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func uniqueTransactionTestTableName() string {
	return "tx_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}
