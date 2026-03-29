package push

import (
	"context"
	"testing"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// fakeWebPushSubscriptionRepo is an in-memory implementation for testing.
type fakeWebPushSubscriptionRepo struct {
	subs []models.WebPushSubscription
}

func (f *fakeWebPushSubscriptionRepo) Save(_ context.Context, userID, endpoint, p256dh, auth string) (*models.WebPushSubscription, error) {
	// Replace existing record with the same endpoint.
	for i, s := range f.subs {
		if s.Endpoint == endpoint {
			f.subs[i].UserID = userID
			f.subs[i].P256DH = p256dh
			f.subs[i].Auth = auth
			return &f.subs[i], nil
		}
	}
	sub := models.WebPushSubscription{
		ID:       "id-" + endpoint,
		UserID:   userID,
		Endpoint: endpoint,
		P256DH:   p256dh,
		Auth:     auth,
	}
	f.subs = append(f.subs, sub)
	return &sub, nil
}

func (f *fakeWebPushSubscriptionRepo) ListByUser(_ context.Context, userID string) ([]models.WebPushSubscription, error) {
	var out []models.WebPushSubscription
	for _, s := range f.subs {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeWebPushSubscriptionRepo) ListAll(_ context.Context) ([]models.WebPushSubscription, error) {
	return append([]models.WebPushSubscription(nil), f.subs...), nil
}

func (f *fakeWebPushSubscriptionRepo) DeleteByEndpoint(_ context.Context, endpoint string) error {
	for i, s := range f.subs {
		if s.Endpoint == endpoint {
			f.subs = append(f.subs[:i], f.subs[i+1:]...)
			return nil
		}
	}
	return repo.ErrNotFound
}

func (f *fakeWebPushSubscriptionRepo) DeleteByUserAndEndpoint(_ context.Context, userID, endpoint string) error {
	for i, s := range f.subs {
		if s.UserID == userID && s.Endpoint == endpoint {
			f.subs = append(f.subs[:i], f.subs[i+1:]...)
			return nil
		}
	}
	return repo.ErrNotFound
}

// --- tests ---

func TestFakeRepo_SaveAndListByUser(t *testing.T) {
	ctx := context.Background()
	r := &fakeWebPushSubscriptionRepo{}

	if _, err := r.Save(ctx, "user1", "https://endpoint1", "key1", "auth1"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := r.Save(ctx, "user1", "https://endpoint2", "key2", "auth2"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := r.Save(ctx, "user2", "https://endpoint3", "key3", "auth3"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	subs, err := r.ListByUser(ctx, "user1")
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(subs) != 2 {
		t.Errorf("expected 2 subscriptions for user1, got %d", len(subs))
	}
}

func TestFakeRepo_SaveUpserts(t *testing.T) {
	ctx := context.Background()
	r := &fakeWebPushSubscriptionRepo{}

	if _, err := r.Save(ctx, "user1", "https://endpoint1", "key1", "auth1"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Save again with same endpoint — should update, not duplicate.
	if _, err := r.Save(ctx, "user1", "https://endpoint1", "key1-updated", "auth1-updated"); err != nil {
		t.Fatalf("Save upsert: %v", err)
	}

	all, _ := r.ListAll(ctx)
	if len(all) != 1 {
		t.Errorf("expected 1 subscription after upsert, got %d", len(all))
	}
	if all[0].P256DH != "key1-updated" {
		t.Errorf("expected updated p256dh, got %q", all[0].P256DH)
	}
}

func TestFakeRepo_DeleteByUserAndEndpoint(t *testing.T) {
	ctx := context.Background()
	r := &fakeWebPushSubscriptionRepo{}

	if _, err := r.Save(ctx, "user1", "https://endpoint1", "key1", "auth1"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := r.DeleteByUserAndEndpoint(ctx, "user1", "https://endpoint1"); err != nil {
		t.Fatalf("DeleteByUserAndEndpoint: %v", err)
	}

	subs, _ := r.ListByUser(ctx, "user1")
	if len(subs) != 0 {
		t.Errorf("expected 0 subscriptions after delete, got %d", len(subs))
	}
}

func TestFakeRepo_DeleteByUserAndEndpoint_NotFound(t *testing.T) {
	ctx := context.Background()
	r := &fakeWebPushSubscriptionRepo{}

	err := r.DeleteByUserAndEndpoint(ctx, "user1", "https://nonexistent")
	if err == nil {
		t.Error("expected ErrNotFound, got nil")
	}
}

func TestFakeRepo_DeleteByEndpoint(t *testing.T) {
	ctx := context.Background()
	r := &fakeWebPushSubscriptionRepo{}

	if _, err := r.Save(ctx, "user1", "https://endpoint1", "key1", "auth1"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := r.Save(ctx, "user2", "https://endpoint2", "key2", "auth2"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := r.DeleteByEndpoint(ctx, "https://endpoint1"); err != nil {
		t.Fatalf("DeleteByEndpoint: %v", err)
	}

	all, _ := r.ListAll(ctx)
	if len(all) != 1 {
		t.Errorf("expected 1 subscription after delete, got %d", len(all))
	}
	if all[0].Endpoint != "https://endpoint2" {
		t.Errorf("expected remaining endpoint2, got %q", all[0].Endpoint)
	}
}

func TestFakeRepo_ListAll(t *testing.T) {
	ctx := context.Background()
	r := &fakeWebPushSubscriptionRepo{}

	if _, err := r.Save(ctx, "user1", "https://ep1", "k1", "a1"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := r.Save(ctx, "user2", "https://ep2", "k2", "a2"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	all, err := r.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 total subscriptions, got %d", len(all))
	}
}
