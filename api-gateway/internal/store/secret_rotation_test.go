package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/govagn/api-gateway/internal/models"
)

// TestRecordSecretRotation verifies rotation logging works correctly.
func TestRecordSecretRotation(t *testing.T) {
	// This test is skipped in CI without a test database
	if os.Getenv("POSTGRES_TEST_URL") == "" && os.Getenv("DATABASE_URL") == "" {
		t.Skip("Skipping database test (set POSTGRES_TEST_URL or DATABASE_URL)")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	record := &models.SecretRotationRecord{
		KeyID:           "master",
		OldKeyHash:      "abcd1234567890abcd1234567890abcd1234567890abcd1234567890abcd1234",
		NewKeyHash:      "dcba9876543210dcba9876543210dcba9876543210dcba9876543210dcba9876",
		ItemsRotated:    42,
		Status:          "success",
		EncryptedByUser: "admin@example.com",
		ErrorMessage:    "",
		DurationSeconds: 15,
	}

	err := db.RecordSecretRotation(ctx, record)
	if err != nil {
		t.Fatalf("RecordSecretRotation failed: %v", err)
	}

	if record.ID == 0 {
		t.Error("Expected non-zero record ID after insert")
	}

	if record.CreatedAt.IsZero() {
		t.Error("Expected non-zero CreatedAt timestamp")
	}
}

// TestGetSecretRotationHistory verifies retrieval of rotation history.
func TestGetSecretRotationHistory(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_URL") == "" && os.Getenv("DATABASE_URL") == "" {
		t.Skip("Skipping database test (set POSTGRES_TEST_URL or DATABASE_URL)")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Insert 3 rotation records
	for i := 0; i < 3; i++ {
		record := &models.SecretRotationRecord{
			KeyID:           "master",
			OldKeyHash:      "hash1",
			NewKeyHash:      "hash2",
			ItemsRotated:    int64(i * 10),
			Status:          "success",
			EncryptedByUser: "user@example.com",
			DurationSeconds: 10 + i,
		}
		err := db.RecordSecretRotation(ctx, record)
		if err != nil {
			t.Fatalf("Failed to insert rotation record: %v", err)
		}
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Retrieve history
	history, err := db.GetSecretRotationHistory(ctx, 10)
	if err != nil {
		t.Fatalf("GetSecretRotationHistory failed: %v", err)
	}

	if len(history) < 3 {
		t.Errorf("Expected at least 3 records, got %d", len(history))
	}

	// Verify newest is first
	if len(history) >= 2 {
		if history[0].CreatedAt.Before(history[1].CreatedAt) {
			t.Error("History should be ordered newest first")
		}
	}
}

// TestGetLastSecretRotation verifies retrieving the most recent rotation.
func TestGetLastSecretRotation(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_URL") == "" && os.Getenv("DATABASE_URL") == "" {
		t.Skip("Skipping database test (set POSTGRES_TEST_URL or DATABASE_URL)")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	record := &models.SecretRotationRecord{
		KeyID:           "master",
		OldKeyHash:      "old_hash",
		NewKeyHash:      "new_hash",
		ItemsRotated:    100,
		Status:          "success",
		EncryptedByUser: "automation@example.com",
		DurationSeconds: 25,
	}

	err := db.RecordSecretRotation(ctx, record)
	if err != nil {
		t.Fatalf("RecordSecretRotation failed: %v", err)
	}

	last, err := db.GetLastSecretRotation(ctx)
	if err != nil {
		t.Fatalf("GetLastSecretRotation failed: %v", err)
	}

	if last.ID != record.ID {
		t.Errorf("Expected ID %d, got %d", record.ID, last.ID)
	}

	if last.KeyID != "master" {
		t.Errorf("Expected key_id 'master', got %q", last.KeyID)
	}

	if last.ItemsRotated != 100 {
		t.Errorf("Expected 100 items rotated, got %d", last.ItemsRotated)
	}
}

// TestSecretRotationAuditTrail verifies the complete audit trail.
func TestSecretRotationAuditTrail(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_URL") == "" && os.Getenv("DATABASE_URL") == "" {
		t.Skip("Skipping database test (set POSTGRES_TEST_URL or DATABASE_URL)")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Record a successful rotation
	successRecord := &models.SecretRotationRecord{
		KeyID:           "master",
		OldKeyHash:      "abc123",
		NewKeyHash:      "def456",
		ItemsRotated:    50,
		Status:          "success",
		EncryptedByUser: "admin@company.com",
		ErrorMessage:    "",
		DurationSeconds: 20,
	}

	err := db.RecordSecretRotation(ctx, successRecord)
	if err != nil {
		t.Fatalf("Failed to record success: %v", err)
	}

	// Record a failed rotation
	failRecord := &models.SecretRotationRecord{
		KeyID:           "master",
		OldKeyHash:      "def456",
		NewKeyHash:      "ghi789",
		ItemsRotated:    0,
		Status:          "failed",
		EncryptedByUser: "admin@company.com",
		ErrorMessage:    "connection timeout",
		DurationSeconds: 45,
	}

	err = db.RecordSecretRotation(ctx, failRecord)
	if err != nil {
		t.Fatalf("Failed to record failure: %v", err)
	}

	// Retrieve history and verify both records are present
	history, err := db.GetSecretRotationHistory(ctx, 100)
	if err != nil {
		t.Fatalf("Failed to retrieve history: %v", err)
	}

	// Find our test records
	var foundSuccess, foundFail bool
	for _, record := range history {
		if record.Status == "success" && record.NewKeyHash == "def456" {
			foundSuccess = true
			if record.ItemsRotated != 50 {
				t.Errorf("Success record: expected 50 items, got %d", record.ItemsRotated)
			}
		}
		if record.Status == "failed" && record.ErrorMessage == "connection timeout" {
			foundFail = true
			if record.ItemsRotated != 0 {
				t.Errorf("Failed record: expected 0 items, got %d", record.ItemsRotated)
			}
		}
	}

	if !foundSuccess {
		t.Error("Success rotation record not found in history")
	}

	if !foundFail {
		t.Error("Failed rotation record not found in history")
	}
}

// Helper to setup test database
func setupTestDB(t *testing.T) *PostgresStore {
	// Use test database if available, otherwise skip
	dbURL := os.Getenv("POSTGRES_TEST_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("No database URL provided")
	}

	db, err := NewPostgresStore(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	return db
}
