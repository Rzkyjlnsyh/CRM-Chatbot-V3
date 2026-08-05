package services

import (
	"strings"
	"testing"
	"time"
)

func TestMetaSecretEncryptionRoundTrip(t *testing.T) {
	t.Setenv("META_CAPI_SECRET_KEY", "unit-test-key-that-is-not-used-in-production")
	encrypted, err := encryptMetaSecret("EAAB-secret-token")
	if err != nil {
		t.Fatalf("encryptMetaSecret: %v", err)
	}
	if encrypted == "EAAB-secret-token" || !strings.HasPrefix(encrypted, "v1:") {
		t.Fatal("token tidak tersimpan dalam format terenkripsi")
	}
	decrypted, err := decryptMetaSecret(encrypted)
	if err != nil || decrypted != "EAAB-secret-token" {
		t.Fatalf("decryptMetaSecret: got %q, err %v", decrypted, err)
	}
}

func TestBuildMetaUserDataNormalizesAndHashesIdentifiers(t *testing.T) {
	data := buildMetaUserData(MetaUserDataInput{
		Email:      "  OWNER@Example.COM ",
		Phone:      "+62 812-3456-7890",
		ExternalID: " 42 ",
		ClientIP:   "203.0.113.10",
		UserAgent:  "test-agent",
		FBP:        "fb.1.123.abc",
		FBC:        "fb.1.123.click",
	})

	assertHash := func(key, normalized string) {
		t.Helper()
		values, ok := data[key].([]string)
		if !ok || len(values) != 1 || values[0] != metaHash(normalized) {
			t.Fatalf("%s tidak di-hash dari nilai ternormalisasi", key)
		}
	}
	assertHash("em", "owner@example.com")
	assertHash("ph", "6281234567890")
	assertHash("external_id", "42")

	if data["client_ip_address"] != "203.0.113.10" || data["client_user_agent"] != "test-agent" {
		t.Fatal("konteks request tidak ikut dibangun")
	}
	if data["fbp"] != "fb.1.123.abc" || data["fbc"] != "fb.1.123.click" {
		t.Fatal("parameter atribusi Meta berubah")
	}
}

func TestMetaRetryDelay(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, time.Minute},
		{1, time.Minute},
		{2, 5 * time.Minute},
		{5, 4 * time.Hour},
		{99, 4 * time.Hour},
	}
	for _, tc := range cases {
		if got := metaRetryDelay(tc.attempt); got != tc.want {
			t.Fatalf("attempt %d: got %s, want %s", tc.attempt, got, tc.want)
		}
	}
}
