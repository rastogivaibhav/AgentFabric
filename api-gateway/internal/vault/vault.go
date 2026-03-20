// Package vault manages virtual LLM API keys.
//
// Real keys are encrypted with AES-256-GCM using a master key loaded from the
// AF_VAULT_KEY environment variable (64-char hex, 32 bytes). The nonce is
// prepended to the ciphertext and stored as a single BYTEA column so the DB
// never holds a plaintext credential.
//
// Virtual keys have the format: af-vk-<32 hex chars>
package vault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors returned by Resolve.
var (
	ErrInvalidKey = errors.New("virtual key not found or revoked")
	ErrKeyExpired = errors.New("virtual key has expired")
)

// KeyInfo is the safe, non-secret view of a virtual key returned by List.
type KeyInfo struct {
	VirtualKey  string
	DisplayName string
	Provider    string
	KeyID       string // first 8 chars of the real key — for display only
	TeamID      string
	CreatedBy   string
	ExpiresAt   *time.Time
	LastUsedAt  *time.Time
	Revoked     bool
	CreatedAt   time.Time
}

// Vault encrypts, stores, and resolves virtual LLM API keys.
type Vault struct {
	pool      *pgxpool.Pool
	masterKey [32]byte
}

// New creates a Vault. masterKeyHex must be a 64-character hex string (32 bytes).
func New(pool *pgxpool.Pool, masterKeyHex string) (*Vault, error) {
	b, err := hex.DecodeString(masterKeyHex)
	if err != nil || len(b) != 32 {
		return nil, fmt.Errorf("AF_VAULT_KEY must be a 64-char hex string (32 bytes): got %d bytes", len(b))
	}
	v := &Vault{pool: pool}
	copy(v.masterKey[:], b)
	return v, nil
}

// Store encrypts realKey and inserts a new virtual key row.
// Returns the generated virtual key string (af-vk-...).
// The real key is never returned after this point.
func (v *Vault) Store(ctx context.Context, tenantID, provider, realKey, displayName string) (string, error) {
	vk, err := generateVirtualKey()
	if err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}

	enc, err := v.encrypt([]byte(realKey))
	if err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}

	keyID := realKey
	if len(keyID) > 8 {
		keyID = keyID[:8]
	}

	_, err = v.pool.Exec(ctx, `
		INSERT INTO virtual_keys
		  (tenant_id, virtual_key, display_name, provider, real_key_enc, key_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, tenantID, vk, displayName, provider, enc, keyID)
	if err != nil {
		return "", fmt.Errorf("insert: %w", err)
	}
	return vk, nil
}

// Resolve looks up virtualKey, decrypts the real key, and stamps last_used_at.
// Returns ErrInvalidKey when the key does not exist or is revoked.
// Returns ErrKeyExpired when expires_at is set and in the past.
func (v *Vault) Resolve(ctx context.Context, virtualKey string) (provider, realKey, tenantID string, err error) {
	var enc []byte
	var expiresAt *time.Time

	err = v.pool.QueryRow(ctx, `
		SELECT provider, real_key_enc, tenant_id, expires_at
		FROM virtual_keys
		WHERE virtual_key = $1 AND NOT revoked
	`, virtualKey).Scan(&provider, &enc, &tenantID, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", "", ErrInvalidKey
		}
		return "", "", "", fmt.Errorf("resolve query: %w", err)
	}

	if expiresAt != nil && time.Now().After(*expiresAt) {
		return "", "", "", ErrKeyExpired
	}

	plain, err := v.decrypt(enc)
	if err != nil {
		return "", "", "", fmt.Errorf("decrypt: %w", err)
	}

	// Stamp last_used_at asynchronously — a failure here is non-fatal.
	go v.pool.Exec(context.Background(), //nolint:errcheck
		`UPDATE virtual_keys SET last_used_at = NOW() WHERE virtual_key = $1`,
		virtualKey,
	)

	return provider, string(plain), tenantID, nil
}

// Revoke marks a virtual key as revoked for the given tenant.
// Returns ErrInvalidKey if the key does not exist or belongs to a different tenant.
func (v *Vault) Revoke(ctx context.Context, virtualKey, tenantID string) error {
	tag, err := v.pool.Exec(ctx, `
		UPDATE virtual_keys SET revoked = true
		WHERE virtual_key = $1 AND tenant_id = $2
	`, virtualKey, tenantID)
	if err != nil {
		return fmt.Errorf("revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidKey
	}
	return nil
}

// List returns all virtual keys for a tenant. Real keys are never included.
func (v *Vault) List(ctx context.Context, tenantID string) ([]KeyInfo, error) {
	rows, err := v.pool.Query(ctx, `
		SELECT virtual_key, display_name, provider, key_id,
		       COALESCE(team_id,''), COALESCE(created_by,''),
		       expires_at, last_used_at, revoked, created_at
		FROM virtual_keys
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()

	var out []KeyInfo
	for rows.Next() {
		var k KeyInfo
		if err := rows.Scan(
			&k.VirtualKey, &k.DisplayName, &k.Provider, &k.KeyID,
			&k.TeamID, &k.CreatedBy,
			&k.ExpiresAt, &k.LastUsedAt, &k.Revoked, &k.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ─── Crypto helpers ───────────────────────────────────────────────────────────

// encrypt seals plaintext with AES-256-GCM.
// Output layout: [ nonce (12 bytes) || ciphertext+tag ]
func (v *Vault) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(v.masterKey[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// Seal appends ciphertext to nonce, so output is nonce||ciphertext.
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt reverses encrypt. Expects nonce prepended to ciphertext.
func (v *Vault) decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(v.masterKey[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, data[:ns], data[ns:], nil)
}

// generateVirtualKey returns a cryptographically random "af-vk-<32 hex chars>" string.
func generateVirtualKey() (string, error) {
	b := make([]byte, 16) // 16 bytes → 32 hex chars
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return "af-vk-" + hex.EncodeToString(b), nil
}
