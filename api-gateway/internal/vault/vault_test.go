package vault

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// devKey is a valid 32-byte (64-char hex) test master key.
const devKey = "0000000000000000000000000000000000000000000000000000000000000001"

func newTestVault(t *testing.T) *Vault {
	t.Helper()
	v, err := New(nil, devKey)
	require.NoError(t, err)
	return v
}

// ─── New ─────────────────────────────────────────────────────────────────────

func TestNew_ValidKey(t *testing.T) {
	_, err := New(nil, devKey)
	assert.NoError(t, err)
}

func TestNew_TooShort(t *testing.T) {
	_, err := New(nil, "deadbeef") // only 4 bytes
	assert.Error(t, err)
}

func TestNew_NotHex(t *testing.T) {
	_, err := New(nil, "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz")
	assert.Error(t, err)
}

// ─── encrypt / decrypt round-trip ────────────────────────────────────────────

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	v := newTestVault(t)
	plaintext := []byte("sk-prod-supersecret-api-key-12345")

	enc, err := v.encrypt(plaintext)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, enc, "ciphertext must not equal plaintext")

	dec, err := v.decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, plaintext, dec)
}

func TestEncryptDecrypt_DifferentNonceEachTime(t *testing.T) {
	v := newTestVault(t)
	plain := []byte("same-key-every-time")

	enc1, _ := v.encrypt(plain)
	enc2, _ := v.encrypt(plain)
	assert.NotEqual(t, enc1, enc2, "each encryption must use a fresh nonce")
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	v := newTestVault(t)
	enc, _ := v.encrypt([]byte("secret"))
	enc[len(enc)-1] ^= 0xFF // flip last byte

	_, err := v.decrypt(enc)
	assert.Error(t, err, "tampered ciphertext must not decrypt")
}

func TestDecrypt_TooShort(t *testing.T) {
	v := newTestVault(t)
	_, err := v.decrypt([]byte{0x01, 0x02}) // shorter than nonce
	assert.Error(t, err)
}

// ─── generateVirtualKey ──────────────────────────────────────────────────────

func TestGenerateVirtualKey_Format(t *testing.T) {
	vk, err := generateVirtualKey()
	require.NoError(t, err)
	assert.True(t, len(vk) == 38, "expected af-vk- (6) + 32 hex = 38 chars, got %d", len(vk))
	assert.Equal(t, "af-vk-", vk[:6])
}

func TestGenerateVirtualKey_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		vk, err := generateVirtualKey()
		require.NoError(t, err)
		assert.False(t, seen[vk], "duplicate virtual key generated")
		seen[vk] = true
	}
}

// ─── Wrong master key ────────────────────────────────────────────────────────

func TestDecrypt_WrongMasterKey(t *testing.T) {
	v1 := newTestVault(t)

	otherKey := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	v2, err := New(nil, otherKey)
	require.NoError(t, err)

	enc, _ := v1.encrypt([]byte("secret"))
	_, err = v2.decrypt(enc)
	assert.Error(t, err, "wrong master key must fail to decrypt")
}
