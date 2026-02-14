package keydir

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/messenger/server/internal/db"
	"github.com/messenger/server/internal/dbenc"
)

// normDomain normalizes localhost-like domains to "local" for comparison.
func normDomain(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "127.0.0.1" || strings.HasPrefix(s, "127.") || s == "localhost" || s == "" {
		return "local"
	}
	return s
}

type Bundle struct {
	IdentityKey    []byte
	SignedPrekey   []byte
	SignedPrekeySig []byte
	SignedPrekeyID  int64
	OneTimePrekey  *PrekeyItem
}

type PrekeyItem struct {
	Key   []byte
	KeyID int64
}

type Service struct {
	db  *db.DB
	dom string
	enc *dbenc.Cipher
}

func New(database *db.DB, domain string, enc *dbenc.Cipher) *Service {
	return &Service{db: database, dom: domain, enc: enc}
}

// GetBundle returns prekey bundle for user@domain/device_id.
// If user is on remote domain, caller (federation) should fetch via S2S.
func (s *Service) GetBundle(ctx context.Context, userID, deviceID string) (*Bundle, error) {
	var username, domain string
	_, err := fmt.Sscanf(userID, "@%[^:]:%s", &username, &domain)
	if err != nil {
		return nil, ErrInvalidUserID
	}
	if normDomain(domain) != normDomain(s.dom) {
		return nil, ErrRemoteUser
	}

	var identityKey, signedPrekey, signedPrekeySig []byte
	var signedPrekeyID int64
	devUUID, err := uuid.Parse(deviceID)
	if err != nil {
		return nil, ErrInvalidDeviceID
	}

	err = s.db.Pool.QueryRow(ctx,
		`SELECT d.identity_key, d.signed_prekey, d.signed_prekey_sig, d.signed_prekey_id
		 FROM devices d
		 JOIN users u ON u.id = d.user_id
		 WHERE LOWER(u.username) = LOWER($1) AND (u.domain = $2 OR (u.domain IN ('localhost','127.0.0.1') AND $2 IN ('localhost','127.0.0.1'))) AND d.id = $3`,
		username, domain, devUUID,
	).Scan(&identityKey, &signedPrekey, &signedPrekeySig, &signedPrekeyID)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if s.enc != nil {
		if identityKey, err = s.enc.Decrypt(identityKey); err != nil {
			return nil, err
		}
		if signedPrekey, err = s.enc.Decrypt(signedPrekey); err != nil {
			return nil, err
		}
		if signedPrekeySig, err = s.enc.Decrypt(signedPrekeySig); err != nil {
			return nil, err
		}
	}

	b := &Bundle{
		IdentityKey:     identityKey,
		SignedPrekey:    signedPrekey,
		SignedPrekeySig: signedPrekeySig,
		SignedPrekeyID:  signedPrekeyID,
	}

	// Try to get one unconsumed one-time prekey
	var pkKey []byte
	var pkKeyID int64
	err = s.db.Pool.QueryRow(ctx,
		`UPDATE one_time_prekeys SET consumed_at = NOW()
		 WHERE id = (SELECT id FROM one_time_prekeys WHERE device_id = $1 AND consumed_at IS NULL ORDER BY key_id LIMIT 1)
		 RETURNING prekey, key_id`,
		devUUID,
	).Scan(&pkKey, &pkKeyID)
	if err == nil {
		if s.enc != nil {
			pkKey, err = s.enc.Decrypt(pkKey)
			if err != nil {
				return nil, err
			}
		}
		b.OneTimePrekey = &PrekeyItem{Key: pkKey, KeyID: pkKeyID}
	}

	return b, nil
}

// GetBundleForUser returns prekey bundle for the first device of a user.
// Used for MVP 1:1 when recipient's device_id is unknown.
// Username is unique, domain is ignored — accepts @user:domain or plain username.
func (s *Service) GetBundleForUser(ctx context.Context, userID string) (*Bundle, string, error) {
	userID = strings.TrimSpace(userID)
	username := strings.TrimPrefix(userID, "@")
	if idx := strings.Index(username, ":"); idx >= 0 {
		username = username[:idx]
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, "", ErrInvalidUserID
	}

	var err error
	var devUUID uuid.UUID
	var identityKey, signedPrekey, signedPrekeySig []byte
	var signedPrekeyID int64
	err = s.db.Pool.QueryRow(ctx,
		`SELECT d.id, d.identity_key, d.signed_prekey, d.signed_prekey_sig, d.signed_prekey_id
		 FROM devices d
		 JOIN users u ON u.id = d.user_id
		 WHERE LOWER(u.username) = LOWER($1)
		 ORDER BY d.created_at ASC LIMIT 1`,
		username,
	).Scan(&devUUID, &identityKey, &signedPrekey, &signedPrekeySig, &signedPrekeyID)
	if err == pgx.ErrNoRows {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if s.enc != nil {
		if identityKey, err = s.enc.Decrypt(identityKey); err != nil {
			return nil, "", err
		}
		if signedPrekey, err = s.enc.Decrypt(signedPrekey); err != nil {
			return nil, "", err
		}
		if signedPrekeySig, err = s.enc.Decrypt(signedPrekeySig); err != nil {
			return nil, "", err
		}
	}

	b := &Bundle{
		IdentityKey:     identityKey,
		SignedPrekey:    signedPrekey,
		SignedPrekeySig: signedPrekeySig,
		SignedPrekeyID:  signedPrekeyID,
	}

	var pkKey []byte
	var pkKeyID int64
	err = s.db.Pool.QueryRow(ctx,
		`UPDATE one_time_prekeys SET consumed_at = NOW()
		 WHERE id = (SELECT id FROM one_time_prekeys WHERE device_id = $1 AND consumed_at IS NULL ORDER BY key_id LIMIT 1)
		 RETURNING prekey, key_id`,
		devUUID,
	).Scan(&pkKey, &pkKeyID)
	if err == nil {
		if s.enc != nil {
			pkKey, err = s.enc.Decrypt(pkKey)
			if err != nil {
				return nil, "", err
			}
		}
		b.OneTimePrekey = &PrekeyItem{Key: pkKey, KeyID: pkKeyID}
	}

	return b, devUUID.String(), nil
}

// UpdateKeys updates signed prekey (if provided) and appends one-time prekeys.
func (s *Service) UpdateKeys(ctx context.Context, userID int64, deviceID uuid.UUID, signedPrekey, signedPrekeySig []byte, signedPrekeyID int64, oneTimePrekeys []PrekeyItem) error {
	if len(signedPrekey) > 0 {
		spk, spkSig := signedPrekey, signedPrekeySig
		if s.enc != nil {
			var err error
			spk, err = s.enc.Encrypt(signedPrekey)
			if err != nil {
				return err
			}
			spkSig, err = s.enc.Encrypt(signedPrekeySig)
			if err != nil {
				return err
			}
		}
		_, err := s.db.Pool.Exec(ctx,
			`UPDATE devices SET signed_prekey = $1, signed_prekey_sig = $2, signed_prekey_id = $3, updated_at = NOW()
			 WHERE user_id = $4 AND id = $5`,
			spk, spkSig, signedPrekeyID, userID, deviceID,
		)
		if err != nil {
			return err
		}
	}

	for _, pk := range oneTimePrekeys {
		pkStored := pk.Key
		if s.enc != nil {
			var encErr error
			pkStored, encErr = s.enc.Encrypt(pk.Key)
			if encErr != nil {
				return encErr
			}
		}
		_, err := s.db.Pool.Exec(ctx,
			`INSERT INTO one_time_prekeys (device_id, key_id, prekey) VALUES ($1, $2, $3)
			 ON CONFLICT (device_id, key_id) DO NOTHING`,
			deviceID, pk.KeyID, pkStored,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

var (
	ErrInvalidUserID  = fmt.Errorf("invalid user id")
	ErrInvalidDeviceID = fmt.Errorf("invalid device id")
	ErrRemoteUser     = fmt.Errorf("user is on remote server")
	ErrNotFound       = fmt.Errorf("not found")
)
