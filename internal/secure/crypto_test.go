package secure

import (
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func testKey() string {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return base64.StdEncoding.EncodeToString(key)
}

func TestCipherRoundTrip(t *testing.T) {
	cipher, err := New(testKey())
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, nonce, err := cipher.Encrypt([]byte(`{"secret":"value"}`))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := cipher.Decrypt(ciphertext, nonce)
	if err != nil || string(plain) != `{"secret":"value"}` {
		t.Fatalf("decrypt = %q, err = %v", plain, err)
	}
}

func TestCipherRejectsTamperedCiphertext(t *testing.T) {
	cipher, err := New(testKey())
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, nonce, _ := cipher.Encrypt([]byte("secret"))
	ciphertext[0] ^= 1
	if _, err := cipher.Decrypt(ciphertext, nonce); err == nil {
		t.Fatal("tampered ciphertext should fail authentication")
	}
}
