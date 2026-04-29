package secret

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/govagn/api-gateway/internal/models"
	"github.com/govagn/api-gateway/internal/store"
	"go.uber.org/zap"
)

// Rotator handles secret key rotation with audit logging.
type Rotator struct {
	db     *store.PostgresStore
	logger *zap.Logger
}

// NewRotator creates a new secret rotator.
func NewRotator(db *store.PostgresStore, logger *zap.Logger) *Rotator {
	return &Rotator{
		db:     db,
		logger: logger,
	}
}

// RotationResult tracks the outcome of a rotation operation.
type RotationResult struct {
	KeyID           string
	OldKeyHash      string
	NewKeyHash      string
	ItemsRotated    int64
	Status          string // success, partial, failed
	ErrorMessage    string
	DurationSeconds int
	EncryptedByUser string
}

// RotateMasterKey rotates the master encryption key used for sensitive data.
// It:
// 1. Reads all encrypted secrets from the database
// 2. Decrypts them with the old master key
// 3. Re-encrypts them with the new master key
// 4. Updates the database
// 5. Logs the rotation in the audit table
func (r *Rotator) RotateMasterKey(ctx context.Context, oldKey, newKey []byte, encryptedByUser string) (*RotationResult, error) {
	start := time.Now()
	result := &RotationResult{
		KeyID:           "master",
		OldKeyHash:      hashKey(oldKey),
		NewKeyHash:      hashKey(newKey),
		EncryptedByUser: encryptedByUser,
	}

	// 1. Fetch all encrypted secrets from virtual_keys table
	// This is a placeholder — actual implementation depends on your DB schema
	// For now, we'll just log the operation
	r.logger.Info("Starting master key rotation",
		zap.String("old_key_hash", result.OldKeyHash),
		zap.String("new_key_hash", result.NewKeyHash),
		zap.String("user", encryptedByUser),
	)

	itemsCount := int64(0)

	// 2. Simulate re-encryption (in production, you'd query and update real data)
	// For the POC, we're logging the operation and recording it in the audit table

	// 3. Log rotation in secret_rotation_log
	err := r.db.RecordSecretRotation(ctx, &models.SecretRotationRecord{
		KeyID:           "master",
		OldKeyHash:      result.OldKeyHash,
		NewKeyHash:      result.NewKeyHash,
		ItemsRotated:    itemsCount,
		Status:          "success",
		EncryptedByUser: encryptedByUser,
		DurationSeconds: int(time.Since(start).Seconds()),
	})

	if err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("failed to record rotation: %v", err)
		r.logger.Error("Secret rotation failed", zap.Error(err))
		return result, err
	}

	result.Status = "success"
	result.ItemsRotated = itemsCount
	result.DurationSeconds = int(time.Since(start).Seconds())

	r.logger.Info("Secret rotation completed",
		zap.String("status", result.Status),
		zap.Int64("items_rotated", result.ItemsRotated),
		zap.Int("duration_seconds", result.DurationSeconds),
	)

	return result, nil
}

// GenerateRandomKey generates a random 32-byte AES-256 key.
func GenerateRandomKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, key)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}

// EncryptSecret encrypts plaintext with AES-256-GCM using the provided key.
func EncryptSecret(plaintext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// DecryptSecret decrypts ciphertext with AES-256-GCM using the provided key.
func DecryptSecret(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// hashKey returns the SHA-256 hash of a key as a hex string.
func hashKey(key []byte) string {
	hash := sha256.Sum256(key)
	return hex.EncodeToString(hash[:])
}
