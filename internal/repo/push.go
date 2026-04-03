package repo

import (
	"context"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// WebPushSubscriptionRepo defines read/write operations for push subscriptions.
type WebPushSubscriptionRepo interface {
	// Save inserts a new subscription, replacing any existing one with the same endpoint.
	Save(ctx context.Context, userID, endpoint, p256dh, auth string) (*models.WebPushSubscription, error)
	// ListByUser returns all subscriptions for a given user.
	ListByUser(ctx context.Context, userID string) ([]models.WebPushSubscription, error)
	// ListAll returns every subscription in the table.
	ListAll(ctx context.Context) ([]models.WebPushSubscription, error)
	// DeleteByEndpoint removes the subscription with the given endpoint.
	DeleteByEndpoint(ctx context.Context, endpoint string) error
	// DeleteByUserAndEndpoint removes the subscription for a specific user + endpoint.
	DeleteByUserAndEndpoint(ctx context.Context, userID, endpoint string) error
	// DeleteByUserAndID removes the subscription with the given ID belonging to userID.
	DeleteByUserAndID(ctx context.Context, userID, id string) error
}

type sqliteWebPushSubscriptionRepo struct {
	db *sqlx.DB
}

// NewWebPushSubscriptionRepo returns a WebPushSubscriptionRepo backed by SQLite.
func NewWebPushSubscriptionRepo(db *sqlx.DB) WebPushSubscriptionRepo {
	return &sqliteWebPushSubscriptionRepo{db: db}
}

func (r *sqliteWebPushSubscriptionRepo) Save(ctx context.Context, userID, endpoint, p256dh, auth string) (*models.WebPushSubscription, error) {
	id := uuid.NewString()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO web_push_subscriptions (id, user_id, endpoint, p256dh, auth)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET
			user_id = excluded.user_id,
			p256dh  = excluded.p256dh,
			auth    = excluded.auth`,
		id, userID, endpoint, p256dh, auth)
	if err != nil {
		return nil, fmt.Errorf("save push subscription: %w", err)
	}
	// Fetch back to return the final record (id may differ on UPSERT path).
	var sub models.WebPushSubscription
	if err := r.db.GetContext(ctx, &sub, `
		SELECT id, user_id, endpoint, p256dh, auth, created_at
		FROM web_push_subscriptions WHERE endpoint = ?`, endpoint); err != nil {
		return nil, fmt.Errorf("fetch saved push subscription: %w", err)
	}
	return &sub, nil
}

func (r *sqliteWebPushSubscriptionRepo) ListByUser(ctx context.Context, userID string) ([]models.WebPushSubscription, error) {
	var subs []models.WebPushSubscription
	if err := r.db.SelectContext(ctx, &subs, `
		SELECT id, user_id, endpoint, p256dh, auth, created_at
		FROM web_push_subscriptions WHERE user_id = ? ORDER BY created_at ASC`, userID); err != nil {
		return nil, fmt.Errorf("list push subscriptions by user: %w", err)
	}
	if subs == nil {
		subs = []models.WebPushSubscription{}
	}
	return subs, nil
}

func (r *sqliteWebPushSubscriptionRepo) ListAll(ctx context.Context) ([]models.WebPushSubscription, error) {
	var subs []models.WebPushSubscription
	if err := r.db.SelectContext(ctx, &subs, `
		SELECT id, user_id, endpoint, p256dh, auth, created_at
		FROM web_push_subscriptions ORDER BY created_at ASC`); err != nil {
		return nil, fmt.Errorf("list all push subscriptions: %w", err)
	}
	if subs == nil {
		subs = []models.WebPushSubscription{}
	}
	return subs, nil
}

func (r *sqliteWebPushSubscriptionRepo) DeleteByEndpoint(ctx context.Context, endpoint string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM web_push_subscriptions WHERE endpoint = ?`, endpoint)
	if err != nil {
		return fmt.Errorf("delete push subscription by endpoint: %w", err)
	}
	return nil
}

func (r *sqliteWebPushSubscriptionRepo) DeleteByUserAndEndpoint(ctx context.Context, userID, endpoint string) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM web_push_subscriptions WHERE user_id = ? AND endpoint = ?`, userID, endpoint)
	if err != nil {
		return fmt.Errorf("delete push subscription: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteWebPushSubscriptionRepo) DeleteByUserAndID(ctx context.Context, userID, id string) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM web_push_subscriptions WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return fmt.Errorf("delete push subscription by id: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
