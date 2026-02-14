package dbenc

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EncryptExisting encrypts all plaintext sensitive columns if cipher is enabled.
// Idempotent: skips already-encrypted values.
func EncryptExisting(ctx context.Context, pool *pgxpool.Pool, c *Cipher) error {
	if c == nil || !c.Enabled() {
		return nil
	}

	// users.password_hash
	rows, err := pool.Query(ctx, `SELECT id, password_hash FROM users WHERE password_hash IS NOT NULL AND length(password_hash) > 0`)
	if err != nil {
		return fmt.Errorf("users: %w", err)
	}
	for rows.Next() {
		var id int64
		var ph []byte
		if err := rows.Scan(&id, &ph); err != nil {
			rows.Close()
			return err
		}
		if IsEncrypted(ph) {
			continue
		}
		enc, err := c.Encrypt(ph)
		if err != nil {
			rows.Close()
			return err
		}
		_, err = pool.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, enc, id)
		if err != nil {
			rows.Close()
			return err
		}
	}
	rows.Close()

	// devices: identity_key, signed_prekey, signed_prekey_sig
	rows, err = pool.Query(ctx, `SELECT id, identity_key, signed_prekey, signed_prekey_sig FROM devices`)
	if err != nil {
		return fmt.Errorf("devices: %w", err)
	}
	for rows.Next() {
		var id interface{}
		var ik, spk, spkSig []byte
		if err := rows.Scan(&id, &ik, &spk, &spkSig); err != nil {
			rows.Close()
			return err
		}
		needEnc := !IsEncrypted(ik) || !IsEncrypted(spk) || !IsEncrypted(spkSig)
		if !needEnc {
			continue
		}
		ikEnc, spkEnc, spkSigEnc := ik, spk, spkSig
		if !IsEncrypted(ik) {
			ikEnc, err = c.Encrypt(ik)
			if err != nil {
				rows.Close()
				return err
			}
		}
		if !IsEncrypted(spk) {
			spkEnc, err = c.Encrypt(spk)
			if err != nil {
				rows.Close()
				return err
			}
		}
		if !IsEncrypted(spkSig) {
			spkSigEnc, err = c.Encrypt(spkSig)
			if err != nil {
				rows.Close()
				return err
			}
		}
		_, err = pool.Exec(ctx, `UPDATE devices SET identity_key = $1, signed_prekey = $2, signed_prekey_sig = $3 WHERE id = $4`,
			ikEnc, spkEnc, spkSigEnc, id)
		if err != nil {
			rows.Close()
			return err
		}
	}
	rows.Close()

	// one_time_prekeys.prekey
	rows, err = pool.Query(ctx, `SELECT id, prekey FROM one_time_prekeys`)
	if err != nil {
		return fmt.Errorf("one_time_prekeys: %w", err)
	}
	for rows.Next() {
		var id int64
		var pk []byte
		if err := rows.Scan(&id, &pk); err != nil {
			rows.Close()
			return err
		}
		if IsEncrypted(pk) {
			continue
		}
		enc, err := c.Encrypt(pk)
		if err != nil {
			rows.Close()
			return err
		}
		_, err = pool.Exec(ctx, `UPDATE one_time_prekeys SET prekey = $1 WHERE id = $2`, enc, id)
		if err != nil {
			rows.Close()
			return err
		}
	}
	rows.Close()

	// access_tokens.token_hash
	rows, err = pool.Query(ctx, `SELECT id, token_hash FROM access_tokens`)
	if err != nil {
		return fmt.Errorf("access_tokens: %w", err)
	}
	for rows.Next() {
		var id interface{}
		var th []byte
		if err := rows.Scan(&id, &th); err != nil {
			rows.Close()
			return err
		}
		if IsEncrypted(th) {
			continue
		}
		enc, err := c.Encrypt(th)
		if err != nil {
			rows.Close()
			return err
		}
		_, err = pool.Exec(ctx, `UPDATE access_tokens SET token_hash = $1 WHERE id = $2`, enc, id)
		if err != nil {
			rows.Close()
			return err
		}
	}
	rows.Close()

	// backup_salts.salt
	rows, err = pool.Query(ctx, `SELECT user_id, salt FROM backup_salts`)
	if err != nil {
		return nil // table might not exist yet
	}
	for rows.Next() {
		var userID int64
		var salt []byte
		if err := rows.Scan(&userID, &salt); err != nil {
			rows.Close()
			return err
		}
		if IsEncrypted(salt) {
			continue
		}
		enc, err := c.Encrypt(salt)
		if err != nil {
			rows.Close()
			return err
		}
		_, err = pool.Exec(ctx, `UPDATE backup_salts SET salt = $1 WHERE user_id = $2`, enc, userID)
		if err != nil {
			rows.Close()
			return err
		}
	}
	rows.Close()

	return nil
}
