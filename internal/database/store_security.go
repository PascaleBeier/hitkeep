package database

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	webauthnlib "github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"hitkeep/internal/api"
	json "hitkeep/internal/jsonapi"
	"hitkeep/internal/security"
)

type PasskeyCredential struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	Name         string
	CredentialID string
	PublicKey    string
	Credential   webauthnlib.Credential
	SignCount    uint32
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type CreateLoginChallengeInput struct {
	UserID       *uuid.UUID
	RememberMe   bool
	Flow         string
	AuthProvider string
}

type LoginChallenge struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	HasUserID    bool
	RememberMe   bool
	Flow         string
	AuthProvider string
	Challenge    string
	Session      *webauthnlib.SessionData
	ExpiresAt    time.Time
}

type DisableUserMFAResult struct {
	TOTPDisabled        bool
	PasskeysDeleted     int
	SessionsInvalidated int
}

type RecoveryCodeStatus struct {
	Generated bool
	Remaining int
}

func (s *Store) HasEnabledTOTP(ctx context.Context, userID uuid.UUID) (bool, error) {
	var count int
	if err := s.QueryRowOrNil(ctx,
		"SELECT COUNT(*) FROM user_totp_factors WHERE user_id = ?",
		[]any{&count},
		userID,
	); err != nil {
		return false, fmt.Errorf("could not check enabled totp factor: %w", err)
	}
	return count > 0, nil
}

func (s *Store) EnableUserTOTP(ctx context.Context, userID uuid.UUID, secret string) error {
	now := time.Now().UTC()
	return s.Transact(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM user_totp_factors WHERE user_id = ?", userID); err != nil {
			return fmt.Errorf("could not remove existing totp factor: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO user_totp_factors (user_id, secret, enabled_at, updated_at) VALUES (?, ?, ?, ?)",
			userID, secret, now, now,
		); err != nil {
			return fmt.Errorf("could not insert enabled totp factor: %w", err)
		}
		return nil
	})
}

func (s *Store) GetUserTOTPSecret(ctx context.Context, userID uuid.UUID) (secret string, found bool, err error) {
	var enabledAt time.Time
	err = s.QueryRowOrNil(ctx,
		"SELECT secret, enabled_at FROM user_totp_factors WHERE user_id = ?",
		[]any{&secret, &enabledAt},
		userID,
	)
	if err != nil {
		return "", false, fmt.Errorf("could not get user totp secret: %w", err)
	}
	if strings.TrimSpace(secret) == "" {
		return "", false, nil
	}
	return secret, true, nil
}

func (s *Store) DisableUserTOTP(ctx context.Context, userID uuid.UUID) error {
	if err := s.Exec(ctx, "DELETE FROM user_totp_factors WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("could not remove enabled totp factor: %w", err)
	}
	return nil
}

func (s *Store) DisableUserMFA(ctx context.Context, userID uuid.UUID) (DisableUserMFAResult, error) {
	var result DisableUserMFAResult

	err := s.Transact(ctx, func(tx *sql.Tx) error {
		totpRes, err := tx.ExecContext(ctx, "DELETE FROM user_totp_factors WHERE user_id = ?", userID)
		if err != nil {
			return fmt.Errorf("could not remove enabled totp factor: %w", err)
		}
		if rows, rowsErr := totpRes.RowsAffected(); rowsErr == nil {
			result.TOTPDisabled = rows > 0
		}

		passkeyRes, err := tx.ExecContext(ctx, "DELETE FROM user_passkeys WHERE user_id = ?", userID)
		if err != nil {
			return fmt.Errorf("could not remove user passkeys: %w", err)
		}
		if rows, rowsErr := passkeyRes.RowsAffected(); rowsErr == nil {
			result.PasskeysDeleted = int(rows)
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM user_recovery_codes WHERE user_id = ?", userID); err != nil {
			return fmt.Errorf("could not remove recovery codes: %w", err)
		}

		sessionRes, err := tx.ExecContext(ctx, "DELETE FROM remember_me_tokens WHERE user_id = ?", userID)
		if err != nil {
			return fmt.Errorf("could not invalidate remember me tokens: %w", err)
		}
		if rows, rowsErr := sessionRes.RowsAffected(); rowsErr == nil {
			result.SessionsInvalidated = int(rows)
		}

		return nil
	})
	if err != nil {
		return DisableUserMFAResult{}, err
	}

	return result, nil
}

func (s *Store) ReplaceUserRecoveryCodes(ctx context.Context, userID uuid.UUID, codeHashes []string) error {
	now := time.Now().UTC()
	return s.Transact(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM user_recovery_codes WHERE user_id = ?", userID); err != nil {
			return fmt.Errorf("could not delete existing recovery codes: %w", err)
		}
		for _, codeHash := range codeHashes {
			if strings.TrimSpace(codeHash) == "" {
				return fmt.Errorf("recovery code hash is required")
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO user_recovery_codes (id, user_id, code_hash, created_at) VALUES (?, ?, ?, ?)",
				uuid.New(), userID, strings.TrimSpace(codeHash), now,
			); err != nil {
				return fmt.Errorf("could not insert recovery code: %w", err)
			}
		}
		return nil
	})
}

func (s *Store) GetRecoveryCodeStatus(ctx context.Context, userID uuid.UUID) (RecoveryCodeStatus, error) {
	var (
		total     int
		remaining int
	)
	if err := s.QueryRowOrNil(ctx,
		"SELECT COUNT(*), COALESCE(SUM(CASE WHEN used_at IS NULL THEN 1 ELSE 0 END), 0) FROM user_recovery_codes WHERE user_id = ?",
		[]any{&total, &remaining},
		userID,
	); err != nil {
		return RecoveryCodeStatus{}, fmt.Errorf("could not load recovery code status: %w", err)
	}
	return RecoveryCodeStatus{
		Generated: total > 0,
		Remaining: remaining,
	}, nil
}

func (s *Store) CountActiveRecoveryCodes(ctx context.Context, userID uuid.UUID) (int, error) {
	status, err := s.GetRecoveryCodeStatus(ctx, userID)
	if err != nil {
		return 0, err
	}
	return status.Remaining, nil
}

func (s *Store) ConsumeRecoveryCode(ctx context.Context, userID uuid.UUID, code string) (int, bool, error) {
	var remaining int
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			"SELECT id, code_hash FROM user_recovery_codes WHERE user_id = ? AND used_at IS NULL",
			userID,
		)
		if err != nil {
			return fmt.Errorf("could not load active recovery codes: %w", err)
		}
		defer rows.Close()

		var matchedID uuid.UUID
		for rows.Next() {
			var (
				id          uuid.UUID
				encodedHash string
			)
			if err := rows.Scan(&id, &encodedHash); err != nil {
				return fmt.Errorf("could not scan recovery code: %w", err)
			}
			match, err := security.VerifyRecoveryCode(code, encodedHash)
			if err != nil {
				return fmt.Errorf("could not verify recovery code: %w", err)
			}
			if match {
				matchedID = id
				break
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("could not iterate recovery codes: %w", err)
		}
		if matchedID == uuid.Nil {
			remaining = -1
			return nil
		}

		result, err := tx.ExecContext(ctx,
			"UPDATE user_recovery_codes SET used_at = ? WHERE id = ? AND user_id = ? AND used_at IS NULL",
			time.Now().UTC(), matchedID, userID,
		)
		if err != nil {
			return fmt.Errorf("could not consume recovery code: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("could not determine consumed recovery code rows: %w", err)
		}
		if affected == 0 {
			remaining = -1
			return nil
		}

		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM user_recovery_codes WHERE user_id = ? AND used_at IS NULL",
			userID,
		).Scan(&remaining); err != nil {
			return fmt.Errorf("could not count remaining recovery codes: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	if remaining < 0 {
		return 0, false, nil
	}
	return remaining, true, nil
}

func (s *Store) CreateUserPasskey(ctx context.Context, userID uuid.UUID, name string, credentialID string, publicKey string, transports []string) (uuid.UUID, error) {
	passkeyID := uuid.New()
	now := time.Now().UTC()

	name = strings.TrimSpace(name)
	if name == "" {
		name = "Passkey"
	}

	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return uuid.Nil, fmt.Errorf("credential id is required")
	}

	var transportsJSON *string
	if len(transports) > 0 {
		payload, err := json.Marshal(transports)
		if err != nil {
			return uuid.Nil, fmt.Errorf("could not encode passkey transports: %w", err)
		}
		value := string(payload)
		transportsJSON = &value
	}

	if err := s.Exec(ctx,
		"INSERT INTO user_passkeys (id, user_id, name, credential_id, public_key, transports_json, sign_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		passkeyID, userID, name, credentialID, strings.TrimSpace(publicKey), transportsJSON, 0, now, now,
	); err != nil {
		return uuid.Nil, fmt.Errorf("could not insert user passkey: %w", err)
	}

	return passkeyID, nil
}

func (s *Store) CreateUserPasskeyCredential(ctx context.Context, userID uuid.UUID, name string, credential webauthnlib.Credential) (uuid.UUID, error) {
	passkeyID := uuid.New()
	now := time.Now().UTC()

	name = strings.TrimSpace(name)
	if name == "" {
		name = "Passkey"
	}

	credentialID := security.EncodeCredentialID(credential.ID)
	if credentialID == "" {
		return uuid.Nil, fmt.Errorf("credential id is required")
	}

	credentialJSON, err := marshalPasskeyCredential(credential)
	if err != nil {
		return uuid.Nil, err
	}

	publicKey := base64.RawURLEncoding.EncodeToString(credential.PublicKey)

	var transportsJSON *string
	if len(credential.Transport) > 0 {
		transports := make([]string, 0, len(credential.Transport))
		for _, transport := range credential.Transport {
			transports = append(transports, string(transport))
		}
		payload, err := json.Marshal(transports)
		if err != nil {
			return uuid.Nil, fmt.Errorf("could not encode passkey transports: %w", err)
		}
		value := string(payload)
		transportsJSON = &value
	}

	if err := s.Exec(ctx,
		"INSERT INTO user_passkeys (id, user_id, name, credential_id, public_key, credential_json, transports_json, sign_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		passkeyID, userID, name, credentialID, publicKey, credentialJSON, transportsJSON, int64(credential.Authenticator.SignCount), now, now,
	); err != nil {
		return uuid.Nil, fmt.Errorf("could not insert user passkey credential: %w", err)
	}

	return passkeyID, nil
}

func (s *Store) ListUserPasskeys(ctx context.Context, userID uuid.UUID) ([]api.UserPasskey, error) {
	passkeys := make([]api.UserPasskey, 0)
	if err := s.QueryList(ctx,
		"SELECT id, name, created_at, updated_at FROM user_passkeys WHERE user_id = ? ORDER BY created_at DESC",
		func(rows *sql.Rows) error {
			var row api.UserPasskey
			if err := rows.Scan(&row.ID, &row.Name, &row.CreatedAt, &row.UpdatedAt); err != nil {
				return fmt.Errorf("could not scan user passkey: %w", err)
			}
			passkeys = append(passkeys, row)
			return nil
		},
		userID,
	); err != nil {
		return nil, fmt.Errorf("could not list user passkeys: %w", err)
	}
	return passkeys, nil
}

func (s *Store) DeleteUserPasskey(ctx context.Context, userID uuid.UUID, passkeyID uuid.UUID) error {
	s.primaryAuthMu.Lock()
	defer s.primaryAuthMu.Unlock()

	return s.Transact(ctx, func(tx *sql.Tx) error {
		var targetCount, otherPasskeyCount, socialCount int
		var passwordLoginEnabled bool
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM user_passkeys WHERE id = ? AND user_id = ?",
			passkeyID, userID,
		).Scan(&targetCount); err != nil {
			return fmt.Errorf("could not check user passkey: %w", err)
		}
		if targetCount == 0 {
			return fmt.Errorf("could not delete user passkey: no rows affected")
		}
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM user_passkeys WHERE user_id = ? AND id <> ?",
			userID, passkeyID,
		).Scan(&otherPasskeyCount); err != nil {
			return fmt.Errorf("could not count alternative passkeys: %w", err)
		}
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM social_identities WHERE user_id = ?",
			userID,
		).Scan(&socialCount); err != nil {
			return fmt.Errorf("could not count alternative social identities: %w", err)
		}
		if err := tx.QueryRowContext(ctx,
			"SELECT COALESCE(password_login_enabled, TRUE) FROM users WHERE id = ?",
			userID,
		).Scan(&passwordLoginEnabled); err != nil {
			return fmt.Errorf("could not load password login state: %w", err)
		}
		if otherPasskeyCount == 0 && socialCount == 0 && !passwordLoginEnabled {
			return ErrLastPrimaryLoginMethod
		}
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM user_passkeys WHERE id = ? AND user_id = ?",
			passkeyID, userID,
		); err != nil {
			return fmt.Errorf("could not delete user passkey: %w", err)
		}
		return nil
	})
}

func (s *Store) GetPasskeyByCredentialID(ctx context.Context, credentialID string) (*PasskeyCredential, error) {
	var row PasskeyCredential
	var signCount int64
	var credentialJSON string
	err := s.QueryRowOrNil(ctx,
		"SELECT id, user_id, name, credential_id, COALESCE(public_key, ''), COALESCE(credential_json, ''), sign_count, created_at, updated_at FROM user_passkeys WHERE credential_id = ?",
		[]any{&row.ID, &row.UserID, &row.Name, &row.CredentialID, &row.PublicKey, &credentialJSON, &signCount, &row.CreatedAt, &row.UpdatedAt},
		strings.TrimSpace(credentialID),
	)
	if err != nil {
		return nil, fmt.Errorf("could not get passkey by credential id: %w", err)
	}
	if row.ID == uuid.Nil {
		return nil, nil
	}
	if signCount < 0 {
		signCount = 0
	}
	if signCount > math.MaxUint32 {
		return nil, fmt.Errorf("passkey sign count is out of range: %d", signCount)
	}
	row.SignCount = uint32(signCount)
	row.Credential, err = parseStoredPasskeyCredential(row.CredentialID, row.PublicKey, credentialJSON, row.SignCount)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) UpdatePasskeySignCount(ctx context.Context, passkeyID uuid.UUID, signCount uint32) error {
	if err := s.ExecRowsAffected(ctx,
		"UPDATE user_passkeys SET sign_count = ?, updated_at = ? WHERE id = ?",
		int64(signCount), time.Now().UTC(), passkeyID,
	); err != nil {
		return fmt.Errorf("could not update passkey sign count: %w", err)
	}
	return nil
}

func (s *Store) UpdatePasskeyCredential(ctx context.Context, passkeyID uuid.UUID, credential webauthnlib.Credential) error {
	credentialJSON, err := marshalPasskeyCredential(credential)
	if err != nil {
		return err
	}

	publicKey := base64.RawURLEncoding.EncodeToString(credential.PublicKey)

	if err := s.ExecRowsAffected(ctx,
		"UPDATE user_passkeys SET public_key = ?, credential_json = ?, sign_count = ?, updated_at = ? WHERE id = ?",
		publicKey, credentialJSON, int64(credential.Authenticator.SignCount), time.Now().UTC(), passkeyID,
	); err != nil {
		return fmt.Errorf("could not update passkey credential: %w", err)
	}
	return nil
}

func (s *Store) ListUserPasskeyCredentials(ctx context.Context, userID uuid.UUID) ([]webauthnlib.Credential, error) {
	credentials := make([]webauthnlib.Credential, 0)
	if err := s.QueryList(ctx,
		"SELECT credential_id, COALESCE(public_key, ''), COALESCE(credential_json, ''), sign_count FROM user_passkeys WHERE user_id = ? ORDER BY created_at DESC",
		func(rows *sql.Rows) error {
			var (
				credentialID   string
				publicKey      string
				credentialJSON string
				signCount      int64
			)
			if err := rows.Scan(&credentialID, &publicKey, &credentialJSON, &signCount); err != nil {
				return fmt.Errorf("could not scan passkey credential: %w", err)
			}
			if signCount < 0 {
				signCount = 0
			}
			if signCount > math.MaxUint32 {
				return fmt.Errorf("passkey sign count is out of range: %d", signCount)
			}
			credential, err := parseStoredPasskeyCredential(credentialID, publicKey, credentialJSON, uint32(signCount))
			if err != nil {
				return err
			}
			credentials = append(credentials, credential)
			return nil
		},
		userID,
	); err != nil {
		return nil, fmt.Errorf("could not list user passkey credentials: %w", err)
	}
	return credentials, nil
}

func marshalPasskeyCredential(credential webauthnlib.Credential) (*string, error) {
	payload, err := json.Marshal(credential)
	if err != nil {
		return nil, fmt.Errorf("could not encode passkey credential: %w", err)
	}
	value := string(payload)
	return &value, nil
}

func parseStoredPasskeyCredential(credentialID string, publicKey string, credentialJSON string, signCount uint32) (webauthnlib.Credential, error) {
	credentialJSON = strings.TrimSpace(credentialJSON)
	if credentialJSON != "" {
		var credential webauthnlib.Credential
		if err := json.Unmarshal([]byte(credentialJSON), &credential); err != nil {
			return webauthnlib.Credential{}, fmt.Errorf("could not decode passkey credential: %w", err)
		}
		return credential, nil
	}

	rawCredentialID, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(credentialID))
	if err != nil {
		return webauthnlib.Credential{}, fmt.Errorf("could not decode legacy credential id: %w", err)
	}

	rawPublicKey, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(publicKey))
	if err != nil {
		return webauthnlib.Credential{}, fmt.Errorf("could not decode legacy passkey public key: %w", err)
	}

	// Legacy passkeys stored a PKIX/DER public key instead of the WebAuthn COSE key.
	// Convert those records on read so existing credentials continue to validate.
	if cosePublicKey, err := convertLegacyPublicKeyToCOSE(rawPublicKey); err == nil {
		rawPublicKey = cosePublicKey
	}

	return webauthnlib.Credential{
		ID:        rawCredentialID,
		PublicKey: rawPublicKey,
		Flags: webauthnlib.CredentialFlags{
			UserPresent:    true,
			UserVerified:   true,
			BackupEligible: false,
			BackupState:    false,
		},
		Authenticator: webauthnlib.Authenticator{
			SignCount: signCount,
		},
		Transport: []protocol.AuthenticatorTransport{},
	}, nil
}

func convertLegacyPublicKeyToCOSE(rawPublicKey []byte) ([]byte, error) {
	publicKey, err := x509.ParsePKIXPublicKey(rawPublicKey)
	if err != nil {
		return nil, err
	}

	switch key := publicKey.(type) {
	case *ecdsa.PublicKey:
		publicKeyBytes, err := key.Bytes()
		if err != nil {
			return nil, fmt.Errorf("encode ecdsa public key: %w", err)
		}
		if len(publicKeyBytes) != 65 || publicKeyBytes[0] != 0x04 {
			return nil, fmt.Errorf("unexpected ecdsa public key encoding length: %d", len(publicKeyBytes))
		}
		return webauthncbor.Marshal(webauthncose.EC2PublicKeyData{
			KeyType:   int64(webauthncose.EllipticKey),
			Algorithm: int64(webauthncose.AlgES256),
			Curve:     int64(webauthncose.P256),
			XCoord:    append([]byte(nil), publicKeyBytes[1:33]...),
			YCoord:    append([]byte(nil), publicKeyBytes[33:65]...),
		})
	case *rsa.PublicKey:
		return webauthncbor.Marshal(webauthncose.RSAPublicKeyData{
			KeyType:   int64(webauthncose.RSAKey),
			Algorithm: int64(webauthncose.AlgRS256),
			Modulus:   append([]byte(nil), key.N.Bytes()...),
			Exponent:  encodeRSAPublicExponent(key.E),
		})
	default:
		return nil, fmt.Errorf("unsupported legacy passkey public key type %T", publicKey)
	}
}

func encodeRSAPublicExponent(exponent int) []byte {
	if exponent <= 0 {
		return nil
	}

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(exponent))
	for len(buf) > 1 && buf[0] == 0 {
		buf = buf[1:]
	}
	return append([]byte(nil), buf...)
}
