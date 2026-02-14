package relay

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/messenger/server/internal/db"
	"github.com/messenger/server/internal/rooms"
)

const defaultTTLDays = 7

type RoomChecker interface {
	IsMember(ctx context.Context, roomID uuid.UUID, userID int64) (bool, error)
	GetRoomAddrsForUser(ctx context.Context, userID int64) ([]string, error)
}

type Service struct {
	db             *db.DB
	federationPool *pgxpool.Pool
	domain         string
	rooms          RoomChecker
}

func New(database *db.DB, domain string, roomChecker RoomChecker) *Service {
	return &Service{db: database, domain: domain, rooms: roomChecker}
}

// SetFederationPool sets optional pool for AcceptTransaction (minimal DB grants)
func (s *Service) SetFederationPool(pool *pgxpool.Pool) {
	s.federationPool = pool
}

func extractDomain(addr string) string {
	if idx := strings.Index(addr, ":"); idx >= 0 && len(addr) > idx+1 {
		return addr[idx+1:]
	}
	return ""
}

type SendInput struct {
	EventID        string
	Sender         string
	Recipient      string
	SenderUserID   int64 // required when recipient is a room
	SenderDevice   string
	Ciphertext     []byte                          // for DM or room legacy
	Ciphertexts    map[string]string                // for room E2EE: user_address -> base64
	SessionID      string
	Timestamp      int64
	IdempotencyKey string
}

func (s *Service) Send(ctx context.Context, in SendInput) (eventID string, err error) {
	if in.EventID == "" {
		in.EventID = "evt_" + uuid.New().String()
	}
	recipientDomain := extractDomain(in.Recipient)
	if recipientDomain != "" && recipientDomain != s.domain {
		return in.EventID, ErrRemoteRecipient
	}

	// Room message: verify sender is member
	if rooms.IsRoomAddr(in.Recipient) && s.rooms != nil {
		roomID, _, ok := rooms.ParseRoomAddr(in.Recipient)
		if !ok {
			return "", fmt.Errorf("invalid room address")
		}
		rid, err := uuid.Parse(roomID)
		if err != nil {
			return "", fmt.Errorf("invalid room id: %w", err)
		}
		member, err := s.rooms.IsMember(ctx, rid, in.SenderUserID)
		if err != nil || !member {
			return "", ErrNotRoomMember
		}
	} else {
		// DM: verify recipient user exists (recipient is local — remote returned above)
		username := strings.TrimPrefix(in.Recipient, "@")
		if idx := strings.Index(username, ":"); idx >= 0 {
			username = username[:idx]
		}
		username = strings.TrimSpace(username)
		if username != "" {
			var exists bool
			err := s.db.Pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(username) = LOWER($1) AND domain = $2)`,
				username, s.domain,
			).Scan(&exists)
			if err != nil {
				return "", fmt.Errorf("check recipient: %w", err)
			}
			if !exists {
				return "", ErrRecipientNotFound
			}
		}
	}

	expiresAt := time.Now().Add(defaultTTLDays * 24 * time.Hour)

	if in.IdempotencyKey != "" {
		keyHash := sha256.Sum256([]byte(in.IdempotencyKey))
		var existing string
		err := s.db.Pool.QueryRow(ctx,
			`SELECT event_id FROM idempotency_keys WHERE key_hash = $1`,
			keyHash[:],
		).Scan(&existing)
		if err == nil {
			return existing, nil
		}
		if err != pgx.ErrNoRows {
			return "", err
		}
	}

	ciphertext := in.Ciphertext
	var ciphertextsJSON []byte
	if len(in.Ciphertexts) > 0 {
		ciphertext = []byte{}
		var errMarshal error
		ciphertextsJSON, errMarshal = json.Marshal(in.Ciphertexts)
		if errMarshal != nil {
			return "", fmt.Errorf("ciphertexts: %w", errMarshal)
		}
	}

	_, err = s.db.Pool.Exec(ctx,
		`INSERT INTO message_queue (event_id, sender, recipient, sender_device, ciphertext, ciphertexts, session_id, timestamp, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		in.EventID, in.Sender, in.Recipient, in.SenderDevice, ciphertext, ciphertextsJSON, in.SessionID, in.Timestamp, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("insert message: %w", err)
	}

	if in.IdempotencyKey != "" {
		keyHash := sha256.Sum256([]byte(in.IdempotencyKey))
		_, _ = s.db.Pool.Exec(ctx,
			`INSERT INTO idempotency_keys (key_hash, event_id) VALUES ($1, $2)`,
			keyHash[:], in.EventID,
		)
	}

	return in.EventID, nil
}

var (
	ErrRemoteRecipient   = fmt.Errorf("recipient is on remote server")
	ErrNotRoomMember     = fmt.Errorf("sender is not a room member")
	ErrRecipientNotFound = fmt.Errorf("recipient user not found")
)

type FederatedEvent struct {
	EventID   string
	Type      string
	Sender    string
	Recipient string
	SenderDevice string
	Ciphertext []byte
	SessionID  string
	Timestamp  int64
}

func (s *Service) AcceptTransaction(ctx context.Context, txnID string, events []FederatedEvent) (accepted []string, rejected []map[string]interface{}) {
	pool := s.db.Pool
	if s.federationPool != nil {
		pool = s.federationPool
	}
	expiresAt := time.Now().Add(defaultTTLDays * 24 * time.Hour)
	for _, e := range events {
		var exists bool
		_ = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM message_queue WHERE event_id = $1)`, e.EventID).Scan(&exists)
		if exists {
			accepted = append(accepted, e.EventID)
			continue
		}
		_, err := pool.Exec(ctx,
			`INSERT INTO message_queue (event_id, sender, recipient, sender_device, ciphertext, session_id, timestamp, expires_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			e.EventID, e.Sender, e.Recipient, e.SenderDevice, e.Ciphertext, e.SessionID, e.Timestamp, expiresAt,
		)
		if err != nil {
			rejected = append(rejected, map[string]interface{}{"event_id": e.EventID, "reason": "insert_failed"})
		} else {
			accepted = append(accepted, e.EventID)
		}
	}
	return accepted, rejected
}

type SyncEvent struct {
	EventID    string
	Type       string
	Sender     string
	Recipient  string
	Timestamp  int64
	Ciphertext []byte
	SessionID  string
	Status     string // queued | delivered
}

// DeliveredUpdate is sent to the message sender when recipient fetches the message.
type DeliveredUpdate struct {
	SenderAddr string
	EventID    string
}

func (s *Service) Sync(ctx context.Context, recipientUserID int64, deviceID uuid.UUID, since string, limit int) (events []SyncEvent, nextCursor string, delivered []DeliveredUpdate, err error) {
	if limit <= 0 {
		limit = 100
	}
	delivered = []DeliveredUpdate{}

	var username, domain string
	err = s.db.Pool.QueryRow(ctx, `SELECT username, domain FROM users WHERE id = $1`, recipientUserID).Scan(&username, &domain)
	if err != nil {
		return nil, "", nil, err
	}
	recipientAddr := "@" + username + ":" + domain

	// recipients = user address + room addresses (if rooms service available)
	recipients := []string{recipientAddr}
	if s.rooms != nil {
		roomAddrs, err := s.rooms.GetRoomAddrsForUser(ctx, recipientUserID)
		if err == nil {
			recipients = append(recipients, roomAddrs...)
		}
	}

	// since = cursor: пустой = первая загрузка (последние N сообщений), иначе — только новые (queued)
	statusFilter := "AND status = 'queued'"
	orderBy := "ORDER BY created_at"
	limitParam := limit + 1
	if since == "" {
		statusFilter = "" // при первой загрузке — вся история
		orderBy = "ORDER BY created_at DESC" // последние сообщения первыми
	}
	// recipient = ANY: входящие (мне или в мои комнаты); sender = recipientAddr: мои исходящие (DM)
	query := `SELECT event_id, msg_type, sender, recipient, timestamp, ciphertext, ciphertexts, session_id, status, created_at
		 FROM message_queue
		 WHERE (recipient = ANY($1::text[]) OR sender = $4) ` + statusFilter + `
		   AND ($2::timestamptz IS NULL OR created_at > $2::timestamptz)
		 ` + orderBy + `
		 LIMIT $3`
	rows, err := s.db.Pool.Query(ctx, query,
		recipients, nullIfEmpty(since), limitParam, recipientAddr,
	)
	if err != nil {
		return nil, "", nil, err
	}
	defer rows.Close()

	var lastCreated time.Time
	var cursorCreated time.Time
	for rows.Next() {
		var e SyncEvent
		var createdAt time.Time
		var ciphertext []byte
		var ciphertextsRaw []byte
		var status string
		err := rows.Scan(&e.EventID, &e.Type, &e.Sender, &e.Recipient, &e.Timestamp, &ciphertext, &ciphertextsRaw, &e.SessionID, &status, &createdAt)
		if err != nil {
			return nil, "", nil, err
		}
		e.Status = status
		e.Ciphertext = ciphertext
		if len(ciphertextsRaw) > 0 {
			var ciphertexts map[string]string
			if json.Unmarshal(ciphertextsRaw, &ciphertexts) == nil {
				if ct, ok := ciphertexts[recipientAddr]; ok && ct != "" {
					if dec, err := base64.StdEncoding.DecodeString(ct); err == nil {
						e.Ciphertext = dec
					}
				}
			}
		}
		events = append(events, e)
		lastCreated = createdAt
		if len(events) == limit {
			cursorCreated = createdAt
		}
	}

	if since == "" && len(events) > 0 {
		// История пришла в DESC — разворачиваем для хронологического порядка
		for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
			events[i], events[j] = events[j], events[i]
		}
	}

	// Pagination: we requested limit+1; if we got more, trim and use cursor from last returned
	if len(events) > limit {
		events = events[:limit]
		nextCursor = cursorCreated.Format(time.RFC3339Nano)
	} else if len(events) > 0 {
		nextCursor = lastCreated.Format(time.RFC3339Nano)
		// Помечаем delivered только входящие; исходящие (sender=я) не трогаем — получатель ещё может не синкать
		// Уведомляем отправителя когда его сообщение доставлено (получатель синкнул)
		for _, ev := range events {
			if ev.Sender != recipientAddr {
				_, _ = s.db.Pool.Exec(ctx, `UPDATE message_queue SET status = 'delivered' WHERE event_id = $1`, ev.EventID)
				delivered = append(delivered, DeliveredUpdate{SenderAddr: ev.Sender, EventID: ev.EventID})
			}
		}
		_, _ = s.db.Pool.Exec(ctx,
			`INSERT INTO sync_cursors (user_id, device_id, cursor) VALUES ($1, $2, $3)
			 ON CONFLICT (user_id, device_id) DO UPDATE SET cursor = $3, updated_at = NOW()`,
			recipientUserID, deviceID, nextCursor,
		)
	}

	return events, nextCursor, delivered, nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// MarkRead records read receipts for events viewed by the reader. Returns sender->event_id pairs to notify.
// For DM: reader must be recipient. For room: reader must be room member (s.rooms used).
func (s *Service) MarkRead(ctx context.Context, readerAddr string, readerUserID int64, eventIDs []string) ([]DeliveredUpdate, error) {
	if len(eventIDs) == 0 {
		return nil, nil
	}
	var toNotify []DeliveredUpdate
	for _, eid := range eventIDs {
		if eid == "" {
			continue
		}
		var sender, recipient string
		err := s.db.Pool.QueryRow(ctx,
			`SELECT sender, recipient FROM message_queue WHERE event_id = $1`,
			eid,
		).Scan(&sender, &recipient)
		if err != nil {
			continue
		}
		canRead := false
		if rooms.IsRoomAddr(recipient) && s.rooms != nil {
			roomIDStr, _, ok := rooms.ParseRoomAddr(recipient)
			if ok {
				rid, err := uuid.Parse(roomIDStr)
				if err == nil {
					member, _ := s.rooms.IsMember(ctx, rid, readerUserID)
					canRead = member
				}
			}
		} else {
			canRead = recipient == readerAddr
		}
		if !canRead {
			continue
		}
		_, err = s.db.Pool.Exec(ctx,
			`INSERT INTO read_receipts (event_id, reader_addr, read_at) VALUES ($1, $2, NOW())
			 ON CONFLICT (event_id, reader_addr) DO NOTHING`,
			eid, readerAddr,
		)
		if err != nil {
			continue
		}
		toNotify = append(toNotify, DeliveredUpdate{SenderAddr: sender, EventID: eid})
	}
	return toNotify, nil
}
