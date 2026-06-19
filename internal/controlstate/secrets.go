package controlstate

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

type EncryptedSecret struct {
	Ciphertext []byte
	Nonce      []byte
	KeyID      string
}

type SecretCipher interface {
	EncryptProviderSecret(plaintext []byte) (*EncryptedSecret, error)
	DecryptProviderSecret(secret *EncryptedSecret) ([]byte, error)
}

type AESGCMSecretCipher struct {
	key   []byte
	keyID string
}

func NewAESGCMSecretCipher(key []byte, keyID string) (*AESGCMSecretCipher, error) {
	if len(key) != 32 {
		return nil, errors.New("AESGCMSecretCipher requires a 32-byte key")
	}
	if keyID == "" {
		return nil, errors.New("AESGCMSecretCipher requires a key ID")
	}
	return &AESGCMSecretCipher{key: key, keyID: keyID}, nil
}

func (c *AESGCMSecretCipher) EncryptProviderSecret(plaintext []byte) (*EncryptedSecret, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	return &EncryptedSecret{
		Ciphertext: ciphertext,
		Nonce:      nonce,
		KeyID:      c.keyID,
	}, nil
}

func (c *AESGCMSecretCipher) DecryptProviderSecret(secret *EncryptedSecret) ([]byte, error) {
	if secret == nil {
		return nil, errors.New("secret is nil")
	}
	if secret.KeyID != c.keyID {
		return nil, errors.New("key ID mismatch")
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := aesgcm.Open(nil, secret.Nonce, secret.Ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
