package rooms

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/messenger/server/internal/db"
)

type Service struct {
	db     *db.DB
	domain string
}

func New(database *db.DB, domain string) *Service {
	return &Service{db: database, domain: domain}
}

// RoomAddr возвращает !room_id:domain
func RoomAddr(roomID uuid.UUID, domain string) string {
	return "!" + roomID.String() + ":" + domain
}

// IsRoomAddr проверяет, что адрес — комната
func IsRoomAddr(addr string) bool {
	return strings.HasPrefix(addr, "!")
}

// ParseRoomAddr возвращает roomID, domain и ok для адреса комнаты !uuid:domain
func ParseRoomAddr(addr string) (roomID string, domain string, ok bool) {
	if !strings.HasPrefix(addr, "!") {
		return "", "", false
	}
	rest := addr[1:]
	if idx := strings.Index(rest, ":"); idx >= 0 && idx+1 < len(rest) {
		return rest[:idx], rest[idx+1:], true
	}
	return "", "", false
}

// Room модель
type Room struct {
	ID        uuid.UUID
	Name      string
	Domain    string
	CreatorID int64
	CreatedAt time.Time
}

// Member участник комнаты
type Member struct {
	UserID   int64
	Username string
	Domain   string
	Role     string
	JoinedAt time.Time
}

// Create создаёт комнату, создатель получает role=creator
func (s *Service) Create(ctx context.Context, name string, creatorUserID int64) (*Room, error) {
	roomID := uuid.New()
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO rooms (id, name, domain, creator_id) VALUES ($1, $2, $3, $4)`,
		roomID, name, s.domain, creatorUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert room: %w", err)
	}
	_, err = s.db.Pool.Exec(ctx,
		`INSERT INTO room_members (room_id, user_id, role) VALUES ($1, $2, 'creator')`,
		roomID, creatorUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("add creator: %w", err)
	}
	return &Room{ID: roomID, Name: name, Domain: s.domain, CreatorID: creatorUserID}, nil
}

// List возвращает комнаты, в которых состоит пользователь
func (s *Service) List(ctx context.Context, userID int64) ([]Room, error) {
	rows, err := s.db.Pool.Query(ctx,
		`SELECT r.id, r.name, r.domain, r.creator_id, r.created_at
		 FROM rooms r
		 JOIN room_members rm ON rm.room_id = r.id
		 WHERE rm.user_id = $1
		 ORDER BY r.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Room
	for rows.Next() {
		var r Room
		if err := rows.Scan(&r.ID, &r.Name, &r.Domain, &r.CreatorID, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

// Get возвращает комнату по ID, только если пользователь — участник
func (s *Service) Get(ctx context.Context, roomID uuid.UUID, userID int64) (*Room, error) {
	var r Room
	err := s.db.Pool.QueryRow(ctx,
		`SELECT r.id, r.name, r.domain, r.creator_id, r.created_at
		 FROM rooms r
		 JOIN room_members rm ON rm.room_id = r.id
		 WHERE r.id = $1 AND rm.user_id = $2`,
		roomID, userID,
	).Scan(&r.ID, &r.Name, &r.Domain, &r.CreatorID, &r.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrNotMember
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// IsMember проверяет, что userID — участник комнаты
func (s *Service) IsMember(ctx context.Context, roomID uuid.UUID, userID int64) (bool, error) {
	var n int
	err := s.db.Pool.QueryRow(ctx,
		`SELECT 1 FROM room_members WHERE room_id = $1 AND user_id = $2`,
		roomID, userID,
	).Scan(&n)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// GetRoomIDsForUser возвращает адреса комнат, в которых состоит пользователь (для Sync)
func (s *Service) GetRoomAddrsForUser(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.db.Pool.Query(ctx,
		`SELECT r.id FROM rooms r
		 JOIN room_members rm ON rm.room_id = r.id
		 WHERE rm.user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var addrs []string
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		addrs = append(addrs, RoomAddr(id, s.domain))
	}
	return addrs, nil
}

// ResolveUserID возвращает user_id по username на домене сервера
func (s *Service) ResolveUserID(ctx context.Context, username string) (int64, error) {
	username = strings.TrimSpace(strings.TrimPrefix(username, "@"))
	if username == "" {
		return 0, fmt.Errorf("username required")
	}
	if idx := strings.Index(username, ":"); idx >= 0 {
		username = username[:idx]
	}
	var id int64
	err := s.db.Pool.QueryRow(ctx,
		`SELECT id FROM users WHERE LOWER(username) = LOWER($1) AND domain = $2`,
		username, s.domain,
	).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, ErrUserNotFound
	}
	return id, err
}

var ErrUserNotFound = fmt.Errorf("user not found")

// Invite добавляет пользователя в комнату (нужны права creator/admin)
func (s *Service) Invite(ctx context.Context, roomID uuid.UUID, inviterUserID, inviteeUserID int64) error {
	role, err := s.getMemberRole(ctx, roomID, inviterUserID)
	if err != nil || (role != "creator" && role != "admin") {
		return ErrForbidden
	}
	_, err = s.db.Pool.Exec(ctx,
		`INSERT INTO room_members (room_id, user_id, role) VALUES ($1, $2, 'member')
		 ON CONFLICT (room_id, user_id) DO NOTHING`,
		roomID, inviteeUserID,
	)
	return err
}

// Leave выйти из комнаты (creator не может выйти, пока не передаст создание)
func (s *Service) Leave(ctx context.Context, roomID uuid.UUID, userID int64) error {
	role, err := s.getMemberRole(ctx, roomID, userID)
	if err != nil {
		return err
	}
	if role == "creator" {
		return ErrCreatorCannotLeave
	}
	_, err = s.db.Pool.Exec(ctx, `DELETE FROM room_members WHERE room_id = $1 AND user_id = $2`, roomID, userID)
	return err
}

// Members список участников
func (s *Service) Members(ctx context.Context, roomID uuid.UUID, requesterUserID int64) ([]Member, error) {
	if _, err := s.Get(ctx, roomID, requesterUserID); err != nil {
		return nil, err
	}
	rows, err := s.db.Pool.Query(ctx,
		`SELECT u.id, u.username, u.domain, rm.role, rm.joined_at
		 FROM room_members rm
		 JOIN users u ON u.id = rm.user_id
		 WHERE rm.room_id = $1
		 ORDER BY rm.role DESC, rm.joined_at`,
		roomID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Username, &m.Domain, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

// SetRole назначает роль (creator/admin только)
func (s *Service) SetRole(ctx context.Context, roomID uuid.UUID, actorUserID, targetUserID int64, newRole string) error {
	if newRole != "admin" && newRole != "member" {
		return fmt.Errorf("invalid role: %s", newRole)
	}
	actorRole, err := s.getMemberRole(ctx, roomID, actorUserID)
	if err != nil || (actorRole != "creator" && actorRole != "admin") {
		return ErrForbidden
	}
	// Только creator может назначать admin
	if newRole == "admin" && actorRole != "creator" {
		return ErrForbidden
	}
	_, err = s.db.Pool.Exec(ctx,
		`UPDATE room_members SET role = $1 WHERE room_id = $2 AND user_id = $3`,
		newRole, roomID, targetUserID,
	)
	return err
}

// TransferCreator передаёт роль creator другому участнику. Только текущий creator может вызвать.
func (s *Service) TransferCreator(ctx context.Context, roomID uuid.UUID, actorUserID, targetUserID int64) error {
	actorRole, err := s.getMemberRole(ctx, roomID, actorUserID)
	if err != nil || actorRole != "creator" {
		return ErrForbidden
	}
	targetRole, err := s.getMemberRole(ctx, roomID, targetUserID)
	if err != nil {
		return err
	}
	if targetUserID == actorUserID {
		return fmt.Errorf("cannot transfer to self")
	}
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE room_members SET role = 'creator' WHERE room_id = $1 AND user_id = $2`, roomID, targetUserID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE room_members SET role = 'member' WHERE room_id = $1 AND user_id = $2`, roomID, actorUserID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE rooms SET creator_id = $1 WHERE id = $2`, targetUserID, roomID)
	if err != nil {
		return err
	}
	_ = tx.Commit(ctx)
	_ = targetRole // unused, target becomes creator
	return nil
}

// Remove исключает участника (creator/admin)
func (s *Service) Remove(ctx context.Context, roomID uuid.UUID, actorUserID, targetUserID int64) error {
	actorRole, err := s.getMemberRole(ctx, roomID, actorUserID)
	if err != nil || (actorRole != "creator" && actorRole != "admin") {
		return ErrForbidden
	}
	targetRole, err := s.getMemberRole(ctx, roomID, targetUserID)
	if err != nil {
		return err
	}
	// Admin не может исключить creator; admin не может исключить другого admin (только creator)
	if targetRole == "creator" || (targetRole == "admin" && actorRole != "creator") {
		return ErrForbidden
	}
	_, err = s.db.Pool.Exec(ctx, `DELETE FROM room_members WHERE room_id = $1 AND user_id = $2`, roomID, targetUserID)
	return err
}

// MemberAddresses returns @username:domain for all members of the room (for internal use e.g. push).
func (s *Service) MemberAddresses(ctx context.Context, roomID uuid.UUID) ([]string, error) {
	rows, err := s.db.Pool.Query(ctx,
		`SELECT u.username, u.domain FROM room_members rm JOIN users u ON u.id = rm.user_id WHERE rm.room_id = $1`,
		roomID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var addrs []string
	for rows.Next() {
		var username, domain string
		if err := rows.Scan(&username, &domain); err != nil {
			continue
		}
		addrs = append(addrs, "@"+username+":"+domain)
	}
	return addrs, nil
}

func (s *Service) getMemberRole(ctx context.Context, roomID uuid.UUID, userID int64) (string, error) {
	var role string
	err := s.db.Pool.QueryRow(ctx,
		`SELECT role FROM room_members WHERE room_id = $1 AND user_id = $2`,
		roomID, userID,
	).Scan(&role)
	if err == pgx.ErrNoRows {
		return "", ErrNotMember
	}
	return role, err
}

var (
	ErrNotMember          = fmt.Errorf("not a room member")
	ErrForbidden          = fmt.Errorf("forbidden")
	ErrCreatorCannotLeave = fmt.Errorf("creator cannot leave without transferring ownership")
)
