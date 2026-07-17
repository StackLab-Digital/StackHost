package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

type Cipher struct {
	gcm cipher.AEAD
}

func New(key string) (*Cipher, error) {
	raw, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("encryption key must be base64 encoded 32 bytes")
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{gcm: gcm}, nil
}

func (c *Cipher) Encrypt(plain []byte) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, c.gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return c.gcm.Seal(nil, nonce, plain, nil), nonce, nil
}

func (c *Cipher) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
	return c.gcm.Open(nil, nonce, ciphertext, nil)
}
