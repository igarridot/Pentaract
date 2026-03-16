package service

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/crypto/pbkdf2"
)

var chunkCipherMagic = []byte{'P', 'T', 'R', 'C', '1'}

const (
	chunkCipherKeyLen         = 32
	chunkCipherKDFIterations  = 600000
	chunkCipherKDFSaltContext = "pentaract/chunk-cipher/v1"
)

// ChunkCipher encrypts and decrypts file chunks transparently.
// Encrypted payload format:
// magic(5) + nonce(12) + gcm(ciphertext+tag).
type ChunkCipher struct {
	aead cipher.AEAD
}

func NewChunkCipher(secret string) *ChunkCipher {
	key := pbkdf2.Key(
		[]byte(secret),
		[]byte(chunkCipherKDFSaltContext),
		chunkCipherKDFIterations,
		chunkCipherKeyLen,
		sha256.New,
	)
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(fmt.Sprintf("creating AES cipher: %v", err))
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(fmt.Sprintf("creating GCM: %v", err))
	}

	return &ChunkCipher{aead: aead}
}

func (c *ChunkCipher) aad(fileID uuid.UUID, position int16) []byte {
	aad := make([]byte, 0, 18)
	aad = append(aad, fileID[:]...)
	var pos [2]byte
	binary.BigEndian.PutUint16(pos[:], uint16(position))
	aad = append(aad, pos[:]...)
	return aad
}

func (c *ChunkCipher) EncryptChunk(fileID uuid.UUID, position int16, plain []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	out := make([]byte, 0, len(chunkCipherMagic)+len(nonce)+len(plain)+c.aead.Overhead())
	out = append(out, chunkCipherMagic...)
	out = append(out, nonce...)
	out = c.aead.Seal(out, nonce, plain, c.aad(fileID, position))
	return out, nil
}

func (c *ChunkCipher) DecryptChunk(fileID uuid.UUID, position int16, payload []byte) ([]byte, error) {
	// Backward compatibility: legacy chunks were stored in plaintext.
	if len(payload) < len(chunkCipherMagic) || !bytes.Equal(payload[:len(chunkCipherMagic)], chunkCipherMagic) {
		return payload, nil
	}

	nonceSize := c.aead.NonceSize()
	if len(payload) < len(chunkCipherMagic)+nonceSize {
		return nil, fmt.Errorf("invalid encrypted payload size")
	}

	nonceOffset := len(chunkCipherMagic)
	nonce := payload[nonceOffset : nonceOffset+nonceSize]
	ciphertext := payload[nonceOffset+nonceSize:]

	plain, err := c.aead.Open(nil, nonce, ciphertext, c.aad(fileID, position))
	if err != nil {
		return nil, fmt.Errorf("decrypting payload: %w", err)
	}
	return plain, nil
}
