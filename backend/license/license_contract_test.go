package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMachineFingerprintUsesPersistentInstallationID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "license", "machine-id")
	t.Setenv("LICENSE_MACHINE_ID_PATH", path)

	first, legacy, err := machineFingerprints()
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := machineFingerprints()
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second || legacy == "" || first == legacy {
		t.Fatalf("unexpected fingerprints current=%q second=%q legacy=%q", first, second, legacy)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("machine ID permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestCallVerifyAPIUsesLMSContractAndStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request verifyRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.LicenseKey != "WA-test" || request.MachineHash != "machine-v2" || request.LegacyMachineHash != "machine-v1" || !request.Heartbeat {
			t.Fatalf("unexpected request: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":"License verified","data":{"valid":true,"status":"active","package_type":"pro","reset_remaining":2,"machines_used":2,"machine_max":3,"message":"OK"}}`))
	}))
	defer server.Close()
	t.Setenv("LICENSE_API_URL", server.URL)
	t.Setenv("LICENSE_API_SECRET", "")

	result := callVerifyAPI("WA-test", "machine-v2", "machine-v1", true)
	if !result.Valid || result.Status != StatusActive || result.PackageType != "pro" || result.ResetRemaining != 2 || result.MachinesUsed != 2 || result.MachineMax != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestCallVerifyAPIVerifiesEd25519Response(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := "WA-signed"
	machine := "machine-signed"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request verifyRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		signedAt := time.Now().Unix()
		keyHash := sha256.Sum256([]byte(key))
		payload := licenseSignaturePayload{
			LicenseKeyHash: hex.EncodeToString(keyHash[:]), MachineHash: machine,
			RequestNonce: request.Nonce, SignedAt: signedAt, Valid: true,
			Status: StatusActive, PackageType: "pro", ResetRemaining: 2,
			MachinesUsed: 1, MachineMax: 3, Message: "OK",
		}
		canonical, _ := json.Marshal(payload)
		response := verifyResponse{
			Valid: true, Status: StatusActive, PackageType: "pro", ResetRemaining: 2,
			MachinesUsed: 1, MachineMax: 3, Message: "OK", SignedAt: signedAt,
			RequestNonce: request.Nonce,
			Signature:    base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(successWrapper{Success: true, Data: &response})
	}))
	defer server.Close()
	t.Setenv("LICENSE_API_URL", server.URL)
	t.Setenv("LICENSE_RESPONSE_SIGNING_PUBLIC_KEY", base64.RawStdEncoding.EncodeToString(publicKey))

	result := callVerifyAPI(key, machine, "", false)
	if !result.Valid || result.Status != StatusActive {
		t.Fatalf("signed response rejected: %+v", result)
	}
}

func TestMachineLimitIsTerminal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"valid":false,"status":"machine_limit_reached","message":"Machine limit reached","machines_used":2,"machine_max":2}}`))
	}))
	defer server.Close()
	t.Setenv("LICENSE_API_URL", server.URL)
	t.Setenv("LICENSE_KEY", "WA-limit")
	t.Setenv("LICENSE_MACHINE_ID_PATH", filepath.Join(t.TempDir(), "machine-id"))

	stateMu.Lock()
	lastHeartbeatAt = time.Now()
	lastHeartbeatOK = true
	stateMu.Unlock()

	if Heartbeat() {
		t.Fatal("machine limit should stop heartbeat")
	}
	stateMu.RLock()
	defer stateMu.RUnlock()
	if VerifyStatus != StatusMachineLimit || VerifyResult {
		t.Fatalf("unexpected state status=%q valid=%v", VerifyStatus, VerifyResult)
	}
}

func TestHeartbeatStopsImmediatelyForRevokedLicense(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"valid":false,"status":"revoked","message":"License has been revoked"}}`))
	}))
	defer server.Close()
	t.Setenv("LICENSE_API_URL", server.URL)
	t.Setenv("LICENSE_KEY", "WA-revoked")
	t.Setenv("LICENSE_MACHINE_ID_PATH", filepath.Join(t.TempDir(), "machine-id"))

	stateMu.Lock()
	lastHeartbeatAt = time.Now()
	lastHeartbeatOK = true
	stateMu.Unlock()

	if Heartbeat() {
		t.Fatal("revoked license should stop heartbeat")
	}
	stateMu.RLock()
	defer stateMu.RUnlock()
	if VerifyStatus != StatusRevoked || VerifyResult {
		t.Fatalf("unexpected state status=%q valid=%v", VerifyStatus, VerifyResult)
	}
}

func TestHeartbeatStopsWhenOfflineGraceExpires(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"success":false,"message":"temporarily unavailable"}`))
	}))
	defer server.Close()
	t.Setenv("LICENSE_API_URL", server.URL)
	t.Setenv("LICENSE_KEY", "WA-offline")
	t.Setenv("LICENSE_OFFLINE_GRACE_HOURS", "1")
	t.Setenv("LICENSE_MACHINE_ID_PATH", filepath.Join(t.TempDir(), "machine-id"))

	stateMu.Lock()
	lastHeartbeatAt = time.Now().Add(-2 * time.Hour)
	lastHeartbeatOK = true
	stateMu.Unlock()

	if Heartbeat() {
		t.Fatal("expired offline grace should stop heartbeat")
	}
	stateMu.RLock()
	defer stateMu.RUnlock()
	if VerifyStatus != StatusGraceExpired || VerifyResult {
		t.Fatalf("unexpected grace state status=%q valid=%v", VerifyStatus, VerifyResult)
	}
}

func TestResetUsesCanonicalLMSContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "machine-id")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request resetRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode reset request: %v", err)
		}
		if request.LicenseKey != "WA-reset" || request.MachineHash == "" {
			t.Fatalf("unexpected reset request: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":"License reset successful"}`))
	}))
	defer server.Close()
	t.Setenv("LICENSE_API_URL", server.URL)
	t.Setenv("LICENSE_KEY", "WA-reset")
	t.Setenv("LICENSE_MACHINE_ID_PATH", path)
	t.Setenv("LICENSE_PUBLIC_RESET_ENABLED", "true")
	if err := Reset(); err != nil {
		t.Fatal(err)
	}
}

func TestInferLegacyStatusDoesNotConfusePackageWithStatus(t *testing.T) {
	tests := []struct {
		valid   bool
		message string
		want    string
	}{
		{true, "OK", StatusActive},
		{false, "License has been revoked", StatusRevoked},
		{false, "License has expired", StatusExpired},
		{false, "Machine mismatch", StatusMachineMismatch},
		{false, "Machine limit reached", StatusMachineLimit},
	}
	for _, test := range tests {
		if got := inferLegacyStatus(test.valid, test.message); got != test.want {
			t.Fatalf("inferLegacyStatus(%v, %q) = %q, want %q", test.valid, test.message, got, test.want)
		}
	}
}
