package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/messenger/server/internal/db"
	"github.com/messenger/server/internal/dbenc"
	"golang.org/x/crypto/bcrypt"
)

const tokenExpiryHours = 24

type Service struct {
	db     *db.DB
	domain string
	jwt    []byte
	enc    *dbenc.Cipher
}

func New(database *db.DB, domain string, jwtSecret []byte, enc *dbenc.Cipher) *Service {
	return &Service{db: database, domain: domain, jwt: jwtSecret, enc: enc}
}

type RegisterInput struct {
	Username        string
	Password        string
	DeviceID        uuid.UUID
	IdentityKey     []byte
	SignedPrekey    []byte
	SignedPrekeySig []byte
	SignedPrekeyID  int64
	OneTimePrekeys  []PrekeyItem
	KeysBackupSalt  []byte // для мульти-устройств
	KeysBackupBlob  []byte
}

type PrekeyItem struct {
	Key   []byte
	KeyID int64
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (userID string, deviceID uuid.UUID, token string, err error) {
	// Check username uniqueness
	var exists bool
	err = s.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE domain = $1 AND username = $2)`,
		s.domain, in.Username,
	).Scan(&exists)
	if err != nil {
		return "", uuid.Nil, "", fmt.Errorf("check username: %w", err)
	}
	if exists {
		return "", uuid.Nil, "", ErrUsernameTaken
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return "", uuid.Nil, "", err
	}
	defer tx.Rollback(ctx)

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return "", uuid.Nil, "", fmt.Errorf("hash password: %w", err)
	}
	phStored := passwordHash
	if s.enc != nil {
		phStored, err = s.enc.Encrypt(passwordHash)
		if err != nil {
			return "", uuid.Nil, "", fmt.Errorf("encrypt password hash: %w", err)
		}
	}

	// Create user
	var uid int64
	err = tx.QueryRow(ctx,
		`INSERT INTO users (username, domain, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		in.Username, s.domain, phStored,
	).Scan(&uid)
	if err != nil {
		return "", uuid.Nil, "", fmt.Errorf("insert user: %w", err)
	}
	userID = fmt.Sprintf("@%s:%s", in.Username, s.domain)

	ikey, spk, spkSig := in.IdentityKey, in.SignedPrekey, in.SignedPrekeySig
	if s.enc != nil {
		if ikey, err = s.enc.Encrypt(in.IdentityKey); err != nil {
			return "", uuid.Nil, "", fmt.Errorf("encrypt identity_key: %w", err)
		}
		if spk, err = s.enc.Encrypt(in.SignedPrekey); err != nil {
			return "", uuid.Nil, "", fmt.Errorf("encrypt signed_prekey: %w", err)
		}
		if spkSig, err = s.enc.Encrypt(in.SignedPrekeySig); err != nil {
			return "", uuid.Nil, "", fmt.Errorf("encrypt signed_prekey_sig: %w", err)
		}
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO devices (id, user_id, identity_key, signed_prekey, signed_prekey_sig, signed_prekey_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		in.DeviceID, uid, ikey, spk, spkSig, in.SignedPrekeyID,
	)
	if err != nil {
		return "", uuid.Nil, "", fmt.Errorf("insert device: %w", err)
	}

	for _, pk := range in.OneTimePrekeys {
		pkStored := pk.Key
		if s.enc != nil {
			pkStored, err = s.enc.Encrypt(pk.Key)
			if err != nil {
				return "", uuid.Nil, "", fmt.Errorf("encrypt prekey: %w", err)
			}
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO one_time_prekeys (device_id, key_id, prekey) VALUES ($1, $2, $3)`,
			in.DeviceID, pk.KeyID, pkStored,
		)
		if err != nil {
			return "", uuid.Nil, "", fmt.Errorf("insert prekey: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", uuid.Nil, "", err
	}

	if len(in.KeysBackupSalt) > 0 && len(in.KeysBackupBlob) > 0 {
		_ = s.StoreKeyBackup(ctx, uid, in.KeysBackupSalt, in.KeysBackupBlob)
	}

	tok, err := s.issueToken(uid, in.DeviceID)
	if err != nil {
		return userID, in.DeviceID, "", err
	}

	th := sha256.Sum256([]byte(tok))
	thStored := th[:]
	if s.enc != nil {
		thStored, err = s.enc.Encrypt(th[:])
		if err != nil {
			return "", uuid.Nil, "", err
		}
	}
	_, _ = s.db.Pool.Exec(ctx,
		`INSERT INTO access_tokens (user_id, device_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, NOW() + INTERVAL '24 hours')`,
		uid, in.DeviceID, thStored,
	)

	return userID, in.DeviceID, tok, nil
}

func (s *Service) issueToken(userID int64, deviceID uuid.UUID) (string, error) {
	claims := jwt.MapClaims{
		"user_id":   userID,
		"device_id": deviceID.String(),
		"exp":       jwt.NewNumericDate(time.Now().Add(tokenExpiryHours * time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwt)
}

func (s *Service) ValidateToken(ctx context.Context, tokenString string) (userID int64, deviceID uuid.UUID, err error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return s.jwt, nil
	})
	if err != nil {
		return 0, uuid.Nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, uuid.Nil, ErrInvalidToken
	}
	uid, _ := claims["user_id"].(float64)
	did, _ := claims["device_id"].(string)
	d, _ := uuid.Parse(did)
	return int64(uid), d, nil
}

type LoginInput struct {
	Username           string
	Password           string
	DeviceID           uuid.UUID
	RequestKeysRestore bool
	IdentityKey        []byte
	SignedPrekey       []byte
	SignedPrekeySig    []byte
	SignedPrekeyID     int64
	OneTimePrekeys     []PrekeyItem
}

type LoginResult struct {
	UserID      string
	DeviceID    uuid.UUID
	Token       string
	KeysBackup  *struct{ Salt, Blob []byte } // для восстановления на новом устройстве
}

func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginResult, error) {
	var uid int64
	var passwordHash []byte
	err := s.db.Pool.QueryRow(ctx,
		`SELECT id, password_hash FROM users WHERE username = $1 AND domain = $2`,
		in.Username, s.domain,
	).Scan(&uid, &passwordHash)
	if err == pgx.ErrNoRows {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if s.enc != nil {
		passwordHash, err = s.enc.Decrypt(passwordHash)
		if err != nil {
			return nil, ErrInvalidCredentials
		}
	}

	// Allow empty hash for migrated users without password (legacy)
	if len(passwordHash) > 0 {
		if err := bcrypt.CompareHashAndPassword(passwordHash, []byte(in.Password)); err != nil {
			return nil, ErrInvalidCredentials
		}
	} else if in.Password != "" {
		return nil, ErrInvalidCredentials
	}

	userID := fmt.Sprintf("@%s:%s", in.Username, s.domain)

	// Check if device exists
	var devExists bool
	err = s.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM devices WHERE user_id = $1 AND id = $2)`,
		uid, in.DeviceID,
	).Scan(&devExists)
	if err != nil {
		return nil, err
	}

	if !devExists {
		var ikeyStored, spkStored, spkSigStored []byte
		spkID := in.SignedPrekeyID
		if len(in.IdentityKey) > 0 {
			ikeyStored, spkStored, spkSigStored = in.IdentityKey, in.SignedPrekey, in.SignedPrekeySig
			if s.enc != nil {
				if ikeyStored, err = s.enc.Encrypt(in.IdentityKey); err != nil {
					return nil, fmt.Errorf("encrypt identity_key: %w", err)
				}
				if spkStored, err = s.enc.Encrypt(in.SignedPrekey); err != nil {
					return nil, fmt.Errorf("encrypt signed_prekey: %w", err)
				}
				if spkSigStored, err = s.enc.Encrypt(in.SignedPrekeySig); err != nil {
					return nil, fmt.Errorf("encrypt signed_prekey_sig: %w", err)
				}
			}
		} else if in.RequestKeysRestore {
			_ = s.db.Pool.QueryRow(ctx,
				`SELECT identity_key, signed_prekey, signed_prekey_sig, signed_prekey_id FROM devices WHERE user_id = $1 LIMIT 1`,
				uid,
			).Scan(&ikeyStored, &spkStored, &spkSigStored, &spkID)
		}
		if len(ikeyStored) == 0 {
			return nil, fmt.Errorf("device keys required for new device")
		}
		_, err = s.db.Pool.Exec(ctx,
			`INSERT INTO devices (id, user_id, identity_key, signed_prekey, signed_prekey_sig, signed_prekey_id)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			in.DeviceID, uid, ikeyStored, spkStored, spkSigStored, spkID,
		)
		if err != nil {
			return nil, fmt.Errorf("add device: %w", err)
		}
		for _, pk := range in.OneTimePrekeys {
			pkStored := pk.Key
			if s.enc != nil {
				pkStored, err = s.enc.Encrypt(pk.Key)
				if err != nil {
					return nil, err
				}
			}
			_, _ = s.db.Pool.Exec(ctx,
				`INSERT INTO one_time_prekeys (device_id, key_id, prekey) VALUES ($1, $2, $3)`,
				in.DeviceID, pk.KeyID, pkStored,
			)
		}
	}

	tok, err := s.issueToken(uid, in.DeviceID)
	if err != nil {
		return nil, err
	}

	th := sha256.Sum256([]byte(tok))
	thStored := th[:]
	if s.enc != nil {
		thStored, err = s.enc.Encrypt(th[:])
		if err != nil {
			return nil, err
		}
	}
	_, _ = s.db.Pool.Exec(ctx,
		`INSERT INTO access_tokens (user_id, device_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, NOW() + INTERVAL '24 hours')`,
		uid, in.DeviceID, thStored,
	)

	res := &LoginResult{UserID: userID, DeviceID: in.DeviceID, Token: tok}
	if in.RequestKeysRestore {
		if salt, blob, e := s.GetKeyBackup(ctx, uid); e == nil {
			res.KeysBackup = &struct{ Salt, Blob []byte }{Salt: salt, Blob: blob}
		}
	}
	return res, nil
}

// ErrUsernameTaken ...
var ErrUsernameTaken = fmt.Errorf("username already taken")

// ErrInvalidToken ...
var ErrInvalidToken = fmt.Errorf("invalid token")

// ErrInvalidCredentials ...
var ErrInvalidCredentials = fmt.Errorf("invalid credentials")

// GetUsername returns username for userID (must be on this domain).
func (s *Service) GetUsername(ctx context.Context, userID int64) (string, error) {
	var username string
	err := s.db.Pool.QueryRow(ctx,
		`SELECT username FROM users WHERE id = $1 AND domain = $2`,
		userID, s.domain,
	).Scan(&username)
	if err != nil {
		return "", err
	}
	return username, nil
}

// StoreKeyBackup saves encrypted private keys for multi-device sync.
// salt and encryptedBundle are stored as-is; client encrypts with KDF(password, salt).
func (s *Service) StoreKeyBackup(ctx context.Context, userID int64, salt, encryptedBundle []byte) error {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	saltStored, bundleStored := salt, encryptedBundle
	if s.enc != nil {
		if saltStored, err = s.enc.Encrypt(salt); err != nil {
			return fmt.Errorf("encrypt salt: %w", err)
		}
		if bundleStored, err = s.enc.Encrypt(encryptedBundle); err != nil {
			return fmt.Errorf("encrypt bundle: %w", err)
		}
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO backup_salts (user_id, salt) VALUES ($1, $2)
		 ON CONFLICT (user_id) DO UPDATE SET salt = $2`,
		userID, saltStored,
	)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO key_backups (user_id, encrypted_bundle) VALUES ($1, $2)
		 ON CONFLICT (user_id) DO UPDATE SET encrypted_bundle = EXCLUDED.encrypted_bundle`,
		userID, bundleStored,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetKeyBackup returns salt and encrypted bundle for key restore on new device.
func (s *Service) GetKeyBackup(ctx context.Context, userID int64) (salt []byte, encryptedBundle []byte, err error) {
	err = s.db.Pool.QueryRow(ctx,
		`SELECT bs.salt, kb.encrypted_bundle
		 FROM backup_salts bs
		 JOIN key_backups kb ON kb.user_id = bs.user_id
		 WHERE bs.user_id = $1`,
		userID,
	).Scan(&salt, &encryptedBundle)
	if err != nil {
		return nil, nil, err
	}
	if s.enc != nil {
		if salt, err = s.enc.Decrypt(salt); err != nil {
			return nil, nil, fmt.Errorf("decrypt salt: %w", err)
		}
		if encryptedBundle, err = s.enc.Decrypt(encryptedBundle); err != nil {
			return nil, nil, fmt.Errorf("decrypt bundle: %w", err)
		}
	}
	return salt, encryptedBundle, nil
}
