package push

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/google/uuid"
	"github.com/messenger/server/internal/db"
)

type Service struct {
	vapidPublic  string
	vapidPrivate string
	db           *db.DB
}

func New(vapidPublic, vapidPrivate string, database *db.DB) *Service {
	return &Service{vapidPublic: vapidPublic, vapidPrivate: vapidPrivate, db: database}
}

func (s *Service) Enabled() bool {
	return s.vapidPublic != "" && s.vapidPrivate != ""
}

func (s *Service) VAPIDPublicKey() string {
	return s.vapidPublic
}

// Subscribe stores a push subscription for the user/device.
func (s *Service) Subscribe(ctx context.Context, userID int64, deviceID uuid.UUID, endpoint, p256dh, auth string) error {
	if endpoint == "" || p256dh == "" || auth == "" {
		return nil
	}
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO push_subscriptions (user_id, device_id, endpoint, p256dh, auth) VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (endpoint) DO UPDATE SET user_id = $1, device_id = $2, p256dh = $4, auth = $5`,
		userID, deviceID, endpoint, p256dh, auth,
	)
	return err
}

// Unsubscribe removes a subscription by endpoint.
func (s *Service) Unsubscribe(ctx context.Context, endpoint string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE endpoint = $1`, endpoint)
	return err
}

// SendToUser sends a push notification to all subscriptions of the user identified by address (@user:domain).
func (s *Service) SendToUser(ctx context.Context, recipientAddr string, payload []byte) {
	if !s.Enabled() || payload == nil {
		return
	}
	// recipientAddr = @username:domain -> get user_id
	recipientAddr = strings.TrimSpace(recipientAddr)
	if !strings.HasPrefix(recipientAddr, "@") {
		return
	}
	parts := strings.SplitN(recipientAddr[1:], ":", 2)
	username := strings.TrimSpace(parts[0])
	domain := ""
	if len(parts) > 1 {
		domain = strings.TrimSpace(parts[1])
	}
	if username == "" {
		return
	}
	var userID int64
	var err error
	if domain != "" {
		err = s.db.Pool.QueryRow(ctx,
			`SELECT id FROM users WHERE LOWER(username) = LOWER($1) AND LOWER(domain) = LOWER($2)`,
			username, domain,
		).Scan(&userID)
	} else {
		err = s.db.Pool.QueryRow(ctx,
			`SELECT id FROM users WHERE LOWER(username) = LOWER($1) LIMIT 1`,
			username,
		).Scan(&userID)
	}
	if err != nil {
		return
	}

	rows, err := s.db.Pool.Query(ctx,
		`SELECT endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	var endpoint, p256dh, auth string
	for rows.Next() {
		if err := rows.Scan(&endpoint, &p256dh, &auth); err != nil {
			continue
		}
		ep, p, a := endpoint, p256dh, auth
		go func() {
			sub := &webpush.Subscription{Endpoint: ep, Keys: webpush.Keys{P256dh: p, Auth: a}}
			resp, err := webpush.SendNotification(payload, sub, &webpush.Options{
				Subscriber:      "messenger",
				VAPIDPublicKey:  s.vapidPublic,
				VAPIDPrivateKey: s.vapidPrivate,
				TTL:             30,
			})
			if err != nil {
				log.Printf("[push] send: %v", err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				log.Printf("[push] status %d for %s", resp.StatusCode, ep[:min(50, len(ep))])
			}
		}()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// NotifyNewMessage builds a JSON payload for "new message" and sends to recipient.
func (s *Service) NotifyNewMessage(ctx context.Context, recipientAddr string, senderLabel string) {
	body := map[string]string{
		"title": "Новое сообщение",
		"body":  senderLabel + ": новое сообщение",
	}
	payload, _ := json.Marshal(body)
	s.SendToUser(ctx, recipientAddr, payload)
}
