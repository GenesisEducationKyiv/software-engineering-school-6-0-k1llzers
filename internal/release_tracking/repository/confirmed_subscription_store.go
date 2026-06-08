package repository

import (
	"context"
	"database/sql"

	releasetracking "github-release-notifier/internal/release_tracking"
)

type ConfirmedSubscriptionStore struct {
	db *sql.DB
}

func NewConfirmedSubscriptionStore(db *sql.DB) *ConfirmedSubscriptionStore {
	return &ConfirmedSubscriptionStore{db: db}
}

func (s *ConfirmedSubscriptionStore) ListConfirmedRepositorySubscriptions(ctx context.Context) ([]releasetracking.ConfirmedRepositorySubscription, error) {
	query := `
		select
			tr.id,
			tr.owner,
			tr.name,
			coalesce(tr.last_seen_tag, ''),
			u.email,
			s.cancellation_token
		from subscriptions s
		join users u on u.id = s.user_id
		join tracked_repositories tr on tr.id = s.tracked_repository_id
		where s.confirmed = true
		order by tr.id, u.email;
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var subscriptions []releasetracking.ConfirmedRepositorySubscription
	for rows.Next() {
		var item releasetracking.ConfirmedRepositorySubscription
		if err := rows.Scan(
			&item.TrackedRepositoryID,
			&item.Owner,
			&item.Name,
			&item.LastSeenTag,
			&item.Email,
			&item.CancellationToken,
		); err != nil {
			return nil, err
		}

		subscriptions = append(subscriptions, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return subscriptions, nil
}
