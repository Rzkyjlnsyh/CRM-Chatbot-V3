package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"

	"wa-assistant/backend/config"
)

// secretbox menyediakan enkripsi at-rest AES-256-GCM untuk rahasia yang disimpan di DB
// (mis. API key AI/embedding). Kunci diturunkan dari SECRET_ENCRYPTION_KEY bila diisi,
// kalau tidak dari JWT_SECRET (selalu ada & wajib kuat). Format keluaran: "v1:<base64(nonce|ciphertext)>".
//
// DecryptSecret kompatibel-mundur: nilai tanpa prefix "v1:" dikembalikan apa adanya,
// sehingga instalasi lama yang menyimpan plaintext tetap terbaca dan otomatis
// ter-enkripsi saat disimpan ulang.

const secretPrefix = "v1:"

func appSecretKey() [32]byte {
	secret := strings.TrimSpace(config.Env("SECRET_ENCRYPTION_KEY", ""))
	if secret == "" {
		secret = strings.TrimSpace(config.EnvRequired("JWT_SECRET")) + "|app-secrets"
	}
	return sha256.Sum256([]byte(secret))
}

// EncryptSecret mengenkripsi value; string kosong tetap dikembalikan kosong.
func EncryptSecret(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	key := appSecretKey()
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(value), nil)
	payload := append(nonce, sealed...)
	return secretPrefix + base64.RawStdEncoding.EncodeToString(payload), nil
}

// DecryptSecret mengembalikan plaintext. Nilai tanpa prefix "v1:" dianggap plaintext lama.
func DecryptSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, secretPrefix) {
		return value, nil // kompatibilitas instalasi lama yang menyimpan plaintext
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, secretPrefix))
	if err != nil {
		return "", err
	}
	key := appSecretKey()
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("secret terenkripsi tidak valid")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
