package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        int64
	Username  string
	Domain    string
	CreatedAt time.Time
}

func (u *User) UserID() string {
	return "@" + u.Username + ":" + u.Domain
}

type Device struct {
	ID              uuid.UUID
	UserID          int64
	IdentityKey     []byte
	SignedPrekey    []byte
	SignedPrekeySig []byte
	SignedPrekeyID  int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type OneTimePrekey struct {
	ID         int64
	DeviceID   uuid.UUID
	KeyID      int64
	Prekey     []byte
	ConsumedAt *time.Time
}

type Message struct {
	ID           uuid.UUID
	EventID      string
	Sender       string
	Recipient    string
	SenderDevice string
	MsgType      string
	Ciphertext   []byte
	SessionID    string
	Timestamp    int64
	Status       string
	ExpiresAt    time.Time
	CreatedAt    time.Time
}
