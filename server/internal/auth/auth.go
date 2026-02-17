package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/messenger/server/internal/db"
	"github.com/messenger/server/internal/dbenc"
	"github.com/messenger/server/internal/keydir"
	"github.com/messenger/server/internal/logjson"
	"golang.org/x/crypto/bcrypt"
)

const tokenExpiryHours = 24
const wsTokenExpirySec = 60

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

	if len(in.OneTimePrekeys) > keydir.MaxOneTimePrekeysPerDevice {
		return "", uuid.Nil, "", fmt.Errorf("one-time prekeys: max %d per device", keydir.MaxOneTimePrekeysPerDevice)
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
	logjson.Log("audit", map[string]interface{}{"action": "device_create", "user_id": uid, "device_id": in.DeviceID.String(), "event": "register"})

	if len(in.KeysBackupSalt) > 0 && len(in.KeysBackupBlob) > 0 {
		_ = s.StoreKeyBackup(ctx, uid, in.KeysBackupSalt, in.KeysBackupBlob)
	}

	tok, jti, err := s.issueToken(uid, in.DeviceID)
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
		`INSERT INTO access_tokens (user_id, device_id, token_hash, jti, expires_at)
		 VALUES ($1, $2, $3, $4, NOW() + INTERVAL '24 hours')`,
		uid, in.DeviceID, thStored, jti,
	)

	return userID, in.DeviceID, tok, nil
}

const tokenAudience = "api"

func (s *Service) issueToken(userID int64, deviceID uuid.UUID) (string, uuid.UUID, error) {
	jti := uuid.New()
	claims := jwt.MapClaims{
		"user_id":   userID,
		"device_id": deviceID.String(),
		"jti":       jti.String(),
		"typ":       "access",
		"iss":       s.domain,
		"aud":       tokenAudience,
		"exp":       jwt.NewNumericDate(time.Now().Add(tokenExpiryHours * time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwt)
	if err != nil {
		return "", uuid.Nil, err
	}
	return signed, jti, nil
}

var jwtParser = jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))

func (s *Service) ValidateToken(ctx context.Context, tokenString string) (userID int64, deviceID uuid.UUID, err error) {
	token, err := jwtParser.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return s.jwt, nil
	})
	if err != nil {
		return 0, uuid.Nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, uuid.Nil, ErrInvalidToken
	}
	jtiStr, _ := claims["jti"].(string)
	if jtiStr == "" {
		return 0, uuid.Nil, ErrInvalidToken // legacy tokens without jti are rejected
	}
	jti, err := uuid.Parse(jtiStr)
	if err != nil {
		return 0, uuid.Nil, ErrInvalidToken
	}
	var exists bool
	err = s.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM access_tokens WHERE jti = $1 AND revoked_at IS NULL AND expires_at > NOW())`,
		jti,
	).Scan(&exists)
	if err != nil || !exists {
		return 0, uuid.Nil, ErrInvalidToken
	}
	if typ, _ := claims["typ"].(string); typ != "" && typ != "access" {
		return 0, uuid.Nil, ErrInvalidToken
	}
	if iss, _ := claims["iss"].(string); iss != "" && iss != s.domain {
		return 0, uuid.Nil, ErrInvalidToken
	}
	if aud, _ := claims["aud"].(string); aud != "" && aud != tokenAudience {
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
		logjson.Log("audit", map[string]interface{}{"action": "device_create", "user_id": uid, "device_id": in.DeviceID.String(), "event": "login_new_device"})
		if len(in.OneTimePrekeys) > keydir.MaxOneTimePrekeysPerDevice {
			return nil, fmt.Errorf("one-time prekeys: max %d per device", keydir.MaxOneTimePrekeysPerDevice)
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

	tok, jti, err := s.issueToken(uid, in.DeviceID)
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
		`INSERT INTO access_tokens (user_id, device_id, token_hash, jti, expires_at)
		 VALUES ($1, $2, $3, $4, NOW() + INTERVAL '24 hours')`,
		uid, in.DeviceID, thStored, jti,
	)

	res := &LoginResult{UserID: userID, DeviceID: in.DeviceID, Token: tok}
	if in.RequestKeysRestore {
		if salt, blob, e := s.GetKeyBackup(ctx, uid); e == nil {
			res.KeysBackup = &struct{ Salt, Blob []byte }{Salt: salt, Blob: blob}
		}
	}
	return res, nil
}

// IssueWSToken returns a short-lived JWT for WebSocket auth (typ=ws, 60s).
func (s *Service) IssueWSToken(userID int64, deviceID uuid.UUID) (string, error) {
	claims := jwt.MapClaims{
		"user_id":   userID,
		"device_id": deviceID.String(),
		"typ":       "ws",
		"iss":       s.domain,
		"aud":       tokenAudience,
		"exp":       jwt.NewNumericDate(time.Now().Add(wsTokenExpirySec * time.Second)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwt)
}

// WSTokenExpirySec returns the ws_token TTL for API response.
func WSTokenExpirySec() int { return wsTokenExpirySec }

// ValidateWSToken validates a WebSocket token (typ=ws). No DB check — short TTL is sufficient.
func (s *Service) ValidateWSToken(tokenString string) (userID int64, deviceID uuid.UUID, err error) {
	token, err := jwtParser.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return s.jwt, nil
	})
	if err != nil {
		return 0, uuid.Nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, uuid.Nil, ErrInvalidToken
	}
	if typ, _ := claims["typ"].(string); typ != "ws" {
		return 0, uuid.Nil, ErrInvalidToken
	}
	if iss, _ := claims["iss"].(string); iss != "" && iss != s.domain {
		return 0, uuid.Nil, ErrInvalidToken
	}
	if aud, _ := claims["aud"].(string); aud != "" && aud != tokenAudience {
		return 0, uuid.Nil, ErrInvalidToken
	}
	uid, _ := claims["user_id"].(float64)
	did, _ := claims["device_id"].(string)
	d, _ := uuid.Parse(did)
	return int64(uid), d, nil
}

// RevokeToken marks the token as revoked.
func (s *Service) RevokeToken(ctx context.Context, tokenString string) error {
	userID, _, err := s.ValidateToken(ctx, tokenString)
	if err != nil {
		return err
	}
	token, err := jwtParser.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return s.jwt, nil
	})
	if err != nil {
		return ErrInvalidToken
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ErrInvalidToken
	}
	jtiStr, _ := claims["jti"].(string)
	if jtiStr == "" {
		return ErrInvalidToken
	}
	jti, err := uuid.Parse(jtiStr)
	if err != nil {
		return ErrInvalidToken
	}
	_, err = s.db.Pool.Exec(ctx,
		`UPDATE access_tokens SET revoked_at = NOW() WHERE jti = $1 AND user_id = $2 AND revoked_at IS NULL`,
		jti, userID,
	)
	return err
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

// DeviceInfo describes a user device for the devices list.
type DeviceInfo struct {
	DeviceID  string
	Name      string
	CreatedAt time.Time
	IsCurrent bool
}

// ListDevices returns devices for the user.
func (s *Service) ListDevices(ctx context.Context, userID int64, currentDeviceID uuid.UUID) ([]DeviceInfo, error) {
	rows, err := s.db.Pool.Query(ctx,
		`SELECT id, COALESCE(name, ''), created_at FROM devices WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeviceInfo
	for rows.Next() {
		var id uuid.UUID
		var name string
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, DeviceInfo{
			DeviceID:  id.String(),
			Name:      name,
			CreatedAt: createdAt,
			IsCurrent: id == currentDeviceID,
		})
	}
	return out, rows.Err()
}

// RenameDevice sets the display name for a device. Only the owner can rename.
func (s *Service) RenameDevice(ctx context.Context, userID int64, deviceID uuid.UUID, name string) error {
	name = sanitizeDeviceName(name)
	_, err := s.db.Pool.Exec(ctx,
		`UPDATE devices SET name = $1, updated_at = NOW() WHERE id = $2 AND user_id = $3`,
		name, deviceID, userID,
	)
	return err
}

func sanitizeDeviceName(s string) string {
	const maxLen = 64
	s = strings.TrimSpace(s)
	var buf strings.Builder
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' || r < 32 {
			continue
		}
		if buf.Len() >= maxLen {
			break
		}
		buf.WriteRune(r)
	}
	return buf.String()
}

// RevokeDevice revokes all tokens for the device and deletes it. The device will need to re-login.
// Deleted devices are no longer returned by keydir.GetBundle/GetBundleForUser/ListDevicesForUser.
func (s *Service) RevokeDevice(ctx context.Context, userID int64, deviceID uuid.UUID) error {
	tag, err := s.db.Pool.Exec(ctx,
		`UPDATE access_tokens SET revoked_at = NOW() WHERE user_id = $1 AND device_id = $2 AND revoked_at IS NULL`,
		userID, deviceID,
	)
	if err != nil {
		return err
	}
	_, err = s.db.Pool.Exec(ctx, `DELETE FROM devices WHERE id = $1 AND user_id = $2`, deviceID, userID)
	if err != nil {
		return err
	}
	_ = tag.RowsAffected()
	return nil
}

// CleanupExpiredTokens removes expired and long-revoked tokens to keep the table bounded.
func (s *Service) CleanupExpiredTokens(ctx context.Context) (deleted int64, err error) {
	res, err := s.db.Pool.Exec(ctx,
		`DELETE FROM access_tokens WHERE expires_at < NOW() OR (revoked_at IS NOT NULL AND revoked_at < NOW() - INTERVAL '7 days')`,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

// ProxySession holds data for a federated login session (user logged in on this server with home server credentials).
type ProxySession struct {
	HomeDomain string
	HomeToken  string
	UserID     string
	DeviceID   string
}

// CreateProxySession stores a proxy session and returns a token the client will use for this server.
func (s *Service) CreateProxySession(ctx context.Context, homeDomain, homeToken, userID, deviceID string, expiresIn time.Duration) (localToken string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	localToken = hex.EncodeToString(b)
	h := sha256.Sum256(b)
	expiresAt := time.Now().Add(expiresIn)
	_, err = s.db.Pool.Exec(ctx,
		`INSERT INTO proxy_sessions (token_hash, home_domain, home_token, user_id, device_id, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		h[:], homeDomain, homeToken, userID, deviceID, expiresAt,
	)
	if err != nil {
		return "", err
	}
	return localToken, nil
}

// GetProxySessionByToken returns proxy session if the token is valid and not expired.
func (s *Service) GetProxySessionByToken(ctx context.Context, token string) (*ProxySession, error) {
	// Token is hex-encoded 32 bytes; we stored sha256(raw bytes), so decode then hash.
	tokBytes, decErr := hex.DecodeString(token)
	if decErr != nil || len(tokBytes) != 32 {
		return nil, ErrInvalidToken
	}
	h := sha256.Sum256(tokBytes)
	var homeDomain, homeToken, userID, deviceID string
	var expiresAt time.Time
	err := s.db.Pool.QueryRow(ctx,
		`SELECT home_domain, home_token, user_id, device_id, expires_at
		 FROM proxy_sessions WHERE token_hash = $1`,
		h[:],
	).Scan(&homeDomain, &homeToken, &userID, &deviceID, &expiresAt)
	if err == pgx.ErrNoRows {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	if time.Now().After(expiresAt) {
		_, _ = s.db.Pool.Exec(ctx, `DELETE FROM proxy_sessions WHERE token_hash = $1`, h[:])
		return nil, ErrInvalidToken
	}
	return &ProxySession{HomeDomain: homeDomain, HomeToken: homeToken, UserID: userID, DeviceID: deviceID}, nil
}
