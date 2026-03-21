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
