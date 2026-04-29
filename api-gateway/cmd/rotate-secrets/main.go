// rotate-secrets is a CLI tool for rotating master encryption keys.
// Usage: rotate-secrets -dry-run=false -new-key /path/to/key -user admin@example.com
//
// This tool:
// 1. Reads a new master key from file (hex-encoded 32-byte AES-256 key)
// 2. Connects to the database
// 3. Queries all encrypted secrets
// 4. Re-encrypts them with the new key
// 5. Updates the database atomically
// 6. Records the rotation in the audit table
//
// The tool supports dry-run mode to verify the operation before execution.

package main

import (
	"context"
	"encoding/hex"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/govagn/api-gateway/internal/secret"
	"github.com/govagn/api-gateway/internal/store"
	"go.uber.org/zap"
)

func main() {
	dryRun := flag.Bool("dry-run", true, "Simulate rotation without committing changes")
	newKeyFile := flag.String("new-key", "", "Path to file containing new master key (hex-encoded)")
	encryptedByUser := flag.String("user", "cli", "User performing the rotation")
	flag.Parse()

	// Setup logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Validate arguments
	if *newKeyFile == "" {
		log.Fatal("--new-key flag is required")
	}

	// Read new key from file
	newKeyHex, err := ioutil.ReadFile(*newKeyFile)
	if err != nil {
		logger.Fatal("Failed to read key file", zap.Error(err), zap.String("file", *newKeyFile))
	}

	// Decode hex to bytes
	newKey, err := hex.DecodeString(string(newKeyHex))
	if err != nil {
		logger.Fatal("Failed to decode key from hex", zap.Error(err))
	}

	if len(newKey) != 32 {
		logger.Fatal("Key must be 32 bytes (256 bits)", zap.Int("got", len(newKey)))
	}

	// Setup database connection
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		logger.Fatal("DATABASE_URL environment variable not set")
	}

	db, err := store.NewPostgresStore(dbURL, logger)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Create rotator
	rotator := secret.NewRotator(db, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if *dryRun {
		logger.Info("DRY-RUN: Simulating rotation",
			zap.String("user", *encryptedByUser),
			zap.Int("new_key_size_bytes", len(newKey)),
		)
		logger.Info("DRY-RUN: Would perform the following operations:")
		logger.Info("1. Query all encrypted secrets from database")
		logger.Info("2. Decrypt each secret with old master key")
		logger.Info("3. Re-encrypt each secret with new master key")
		logger.Info("4. Batch update database")
		logger.Info("5. Record rotation in secret_rotation_log")
		logger.Info("Run again with -dry-run=false to execute the rotation")
		return
	}

	logger.Info("Starting secret key rotation",
		zap.String("user", *encryptedByUser),
	)

	// Perform rotation
	result, err := rotator.RotateMasterKey(ctx, make([]byte, 32), newKey, *encryptedByUser)
	if err != nil {
		logger.Error("Rotation failed", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("Secret rotation completed",
		zap.String("status", result.Status),
		zap.Int64("items_rotated", result.ItemsRotated),
		zap.Int("duration_seconds", result.DurationSeconds),
		zap.String("new_key_hash", result.NewKeyHash),
	)

	// Verify rotation
	history, err := db.GetSecretRotationHistory(ctx, 5)
	if err != nil {
		logger.Error("Failed to retrieve rotation history", zap.Error(err))
		os.Exit(1)
	}

	if len(history) > 0 {
		recent := history[0]
		logger.Info("Latest rotation record",
			zap.Int64("id", recent.ID),
			zap.String("status", recent.Status),
			zap.Time("created_at", recent.CreatedAt),
		)
	}
}
