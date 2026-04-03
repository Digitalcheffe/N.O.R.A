package push

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// pushPayload is the JSON body delivered to the browser service worker.
type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Icon  string `json:"icon"`
}

// Sender delivers Web Push notifications using VAPID.
type Sender struct {
	cfg   *config.Config
	store *repo.Store
}

// NewSender creates a Sender.
func NewSender(cfg *config.Config, store *repo.Store) *Sender {
	return &Sender{cfg: cfg, store: store}
}

// SendToUser sends a push notification to all subscriptions owned by userID.
func (s *Sender) SendToUser(ctx context.Context, userID, title, body string) error {
	subs, err := s.store.WebPushSubscriptions.ListByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("SendToUser: list subscriptions: %w", err)
	}
	s.sendToSubscriptions(ctx, subs, title, body)
	return nil
}

// SendToAll sends a push notification to every stored subscription.
func (s *Sender) SendToAll(ctx context.Context, title, body string) error {
	subs, err := s.store.WebPushSubscriptions.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("SendToAll: list subscriptions: %w", err)
	}
	s.sendToSubscriptions(ctx, subs, title, body)
	return nil
}

// sendToSubscriptions delivers a notification to a slice of subscriptions.
// Stale subscriptions (HTTP 410) are removed. Other errors are logged and skipped.
func (s *Sender) sendToSubscriptions(ctx context.Context, subs []models.WebPushSubscription, title, body string) {
	payload, err := json.Marshal(pushPayload{
		Title: title,
		Body:  body,
		Icon:  "/icons/icon-192.png",
	})
	if err != nil {
		log.Printf("push: marshal payload: %v", err)
		return
	}

	for _, sub := range subs {
		resp, err := webpush.SendNotification(payload, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				P256dh: sub.P256DH,
				Auth:   sub.Auth,
			},
		}, &webpush.Options{
			Subscriber:      s.cfg.VAPIDSubject,
			VAPIDPublicKey:  s.cfg.VAPIDPublic,
			VAPIDPrivateKey: s.cfg.VAPIDPrivate,
			TTL:             86400,
			Urgency:         webpush.UrgencyHigh,
		})
		if err != nil {
			log.Printf("push: send to %s: %v", sub.Endpoint, err)
			continue
		}

		if resp.StatusCode == http.StatusGone {
			// Browser has unsubscribed — clean up the stale record.
			resp.Body.Close()
			log.Printf("push: subscription gone, removing endpoint %s", sub.Endpoint)
			if delErr := s.store.WebPushSubscriptions.DeleteByEndpoint(ctx, sub.Endpoint); delErr != nil {
				log.Printf("push: delete stale subscription: %v", delErr)
			}
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			log.Printf("push: unexpected status %d for endpoint %s: %s", resp.StatusCode, sub.Endpoint, bodyBytes)
			continue
		}
		resp.Body.Close()
	}
}
