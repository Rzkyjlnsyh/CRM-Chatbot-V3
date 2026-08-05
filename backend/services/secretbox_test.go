package services

import (
	"strings"
	"testing"
)

const testEncKey = "unit-test-secret-encryption-key-32b!"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", testEncKey)
	plain := "sk-abcdef1234567890XYZ"
	enc, err := EncryptSecret(plain)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	if enc == plain {
		t.Fatal("ciphertext tidak boleh sama dengan plaintext")
	}
	if !strings.HasPrefix(enc, "v1:") {
		t.Fatalf("hasil enkripsi harus berprefix v1:, dapat %q", enc)
	}
	got, err := DecryptSecret(enc)
	if err != nil {
		t.Fatalf("DecryptSecret: %v", err)
	}
	if got != plain {
		t.Fatalf("round-trip gagal: %q != %q", got, plain)
	}
}

func TestEncryptEmptyStaysEmpty(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", testEncKey)
	enc, err := EncryptSecret("")
	if err != nil {
		t.Fatal(err)
	}
	if enc != "" {
		t.Fatalf("string kosong harus tetap kosong, dapat %q", enc)
	}
}

func TestDecryptPlaintextBackwardCompat(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", testEncKey)
	got, err := DecryptSecret("plaintext-lama-tanpa-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if got != "plaintext-lama-tanpa-prefix" {
		t.Fatalf("nilai plaintext lama harus dikembalikan apa adanya, dapat %q", got)
	}
}

func TestEncryptUsesRandomNonce(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", testEncKey)
	a, _ := EncryptSecret("nilai-sama")
	b, _ := EncryptSecret("nilai-sama")
	if a == b {
		t.Fatal("dua enkripsi untuk nilai sama harus berbeda (nonce acak)")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", testEncKey)
	enc, err := EncryptSecret("rahasia")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SECRET_ENCRYPTION_KEY", "kunci-berbeda-yang-tidak-cocok-123")
	if _, err := DecryptSecret(enc); err == nil {
		t.Fatal("dekripsi dengan kunci berbeda seharusnya gagal")
	}
}
