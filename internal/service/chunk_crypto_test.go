package service

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
)

func TestChunkCipherEncryptDecryptRoundTrip(t *testing.T) {
	c := NewChunkCipher("test-secret")
	fileID := uuid.New()
	pos := int16(3)
	plain := []byte("hello telegram encrypted world")

	enc, release, err := c.EncryptChunk(fileID, pos, plain)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	defer release()
	if bytes.Equal(enc, plain) {
		t.Fatalf("encrypted payload should differ from plaintext")
	}

	dec, err := c.DecryptChunk(fileID, pos, enc)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("unexpected decrypted payload: got %q want %q", dec, plain)
	}
}

func TestChunkCipherDecryptLegacyPlaintext(t *testing.T) {
	c := NewChunkCipher("test-secret")
	fileID := uuid.New()
	pos := int16(0)
	plain := []byte("legacy unencrypted chunk")

	dec, err := c.DecryptChunk(fileID, pos, plain)
	if err != nil {
		t.Fatalf("decrypt legacy chunk failed: %v", err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("legacy plaintext should pass through unchanged")
	}
}

func TestChunkCipherRejectsWrongAAD(t *testing.T) {
	c := NewChunkCipher("test-secret")
	fileID := uuid.New()
	otherFileID := uuid.New()
	pos := int16(1)
	plain := []byte("secret")

	enc, release, err := c.EncryptChunk(fileID, pos, plain)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	defer release()

	if _, err := c.DecryptChunk(otherFileID, pos, enc); err == nil {
		t.Fatalf("expected decrypt to fail with different file ID")
	}
}

func TestChunkCipherFallbackOpensChunksSealedWithLegacySecret(t *testing.T) {
	fileID := uuid.New()
	legacy := NewChunkCipher("old-secret-key")
	sealedWithLegacy, _, err := legacy.EncryptChunk(fileID, 3, []byte("written before the key change"))
	if err != nil {
		t.Fatalf("encrypt with legacy: %v", err)
	}

	rotated := NewChunkCipherWithFallback("new-encryption-key", "old-secret-key")
	plain, err := rotated.DecryptChunk(fileID, 3, sealedWithLegacy)
	if err != nil || string(plain) != "written before the key change" {
		t.Fatalf("expected legacy chunk to decrypt, got %q %v", plain, err)
	}

	sealedWithNew, _, err := rotated.EncryptChunk(fileID, 4, []byte("written after"))
	if err != nil {
		t.Fatalf("encrypt with rotated: %v", err)
	}
	if _, err := legacy.DecryptChunk(fileID, 4, sealedWithNew); err == nil {
		t.Fatalf("new chunks must be sealed with the new key only")
	}
	plain, err = rotated.DecryptChunk(fileID, 4, sealedWithNew)
	if err != nil || string(plain) != "written after" {
		t.Fatalf("expected new chunk to decrypt with the primary key, got %q %v", plain, err)
	}
}

func TestChunkCipherFallbackIgnoresEmptyOrIdenticalLegacySecret(t *testing.T) {
	if c := NewChunkCipherWithFallback("s", ""); c.legacy != nil {
		t.Fatalf("empty legacy secret must not add a fallback")
	}
	if c := NewChunkCipherWithFallback("s", "s"); c.legacy != nil {
		t.Fatalf("identical legacy secret must not add a fallback")
	}
	wrong := NewChunkCipherWithFallback("a", "b")
	other := NewChunkCipher("c")
	sealed, _, _ := other.EncryptChunk(uuid.New(), 0, []byte("x"))
	if _, err := wrong.DecryptChunk(uuid.New(), 0, sealed); err == nil {
		t.Fatalf("a chunk sealed with an unrelated key must still fail")
	}
}
