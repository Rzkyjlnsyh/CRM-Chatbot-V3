package license

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"wa-assistant/backend/config"
)

const (
	StatusActive           = "active"
	StatusRevoked          = "revoked"
	StatusExpired          = "expired"
	StatusMachineMismatch  = "machine_mismatch"
	StatusMachineLimit     = "machine_limit_reached"
	StatusNotFound         = "not_found"
	StatusNetworkError     = "network_error"
	StatusRateLimited      = "rate_limited"
	StatusServerError      = "server_error"
	StatusUnauthorized     = "unauthorized"
	StatusInvalidRequest   = "invalid_request"
	StatusGraceExpired     = "offline_grace_expired"
	StatusInvalidSignature = "invalid_signature"
)

type verifyRequest struct {
	LicenseKey        string `json:"license_key"`
	MachineHash       string `json:"machine_hash"`
	LegacyMachineHash string `json:"legacy_machine_hash,omitempty"`
	Heartbeat         bool   `json:"heartbeat,omitempty"`
	Nonce             string `json:"nonce,omitempty"`
}

type verifyResponse struct {
	Valid          bool   `json:"valid"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	PackageType    string `json:"package_type"`
	ResetRemaining int    `json:"reset_remaining"`
	MachinesUsed   int    `json:"machines_used"`
	MachineMax     int    `json:"machine_max"`
	SignedAt       int64  `json:"signed_at,omitempty"`
	RequestNonce   string `json:"request_nonce,omitempty"`
	Signature      string `json:"signature,omitempty"`
	SignatureKeyID string `json:"signature_key_id,omitempty"`
}

type successWrapper struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    *verifyResponse `json:"data"`
}

type verificationResult struct {
	Valid          bool
	Status         string
	Message        string
	PackageType    string
	ResetRemaining int
	MachinesUsed   int
	MachineMax     int
}

// Set these at build time for production. This prevents a user-editable .env
// from changing the LMS endpoint or replacing the trusted signing key.
// -ldflags "-X wa-assistant/backend/license.PinnedLicenseAPIURL=https://api.slaludiskon.com -X wa-assistant/backend/license.PinnedLicenseSigningPublicKey=<base64>"
var (
	PinnedLicenseAPIURL           string
	PinnedLicenseSigningPublicKey string
)

var (
	stateMu sync.RWMutex

	VerifyResult      bool
	VerifyMessage     string
	VerifyStatus      string
	VerifyPackageType string
	lastHeartbeatOK   = true
	lastHeartbeatAt   time.Time
	heartbeatFailCnt  int

	licenseHTTPClient = &http.Client{Timeout: 15 * time.Second}
)

// DevMode can only be enabled at build time with:
// -ldflags "-X wa-assistant/backend/license.DevMode=true".
var DevMode = "false"

// Verify validates and binds the license before the application starts.
func Verify() bool {
	if DevMode == "true" {
		setVerificationState(true, "dev", "dev mode", "")
		log.Println("[license] DEV MODE — skip verifikasi lisensi")
		return true
	}

	key := strings.TrimSpace(config.Env("LICENSE_KEY", ""))
	if key == "" {
		setVerificationState(false, "no_key", "LICENSE_KEY kosong. Beli lisensi di slaludiskon.com", "")
		log.Printf("[license] GAGAL: %s", VerifyMessage)
		return false
	}

	machine, legacyMachine, err := machineFingerprints()
	if err != nil {
		setVerificationState(false, "machine_id_error", fmt.Sprintf("gagal menyiapkan identitas instalasi: %v", err), "")
		log.Printf("[license] GAGAL: %s", VerifyMessage)
		return false
	}

	log.Printf("[license] Verifying key=%s machine=%s...", maskKey(key), machine[:8])
	result := callVerifyAPI(key, machine, legacyMachine, false)
	setVerificationState(result.Valid, result.Status, result.Message, result.PackageType)
	if !result.Valid {
		log.Printf("[license] VERIFY GAGAL status=%s: %s", result.Status, result.Message)
		return false
	}

	stateMu.Lock()
	lastHeartbeatOK = true
	lastHeartbeatAt = time.Now()
	heartbeatFailCnt = 0
	stateMu.Unlock()
	log.Printf("[license] VERIFY OK — status=%s package=%s", result.Status, result.PackageType)
	return true
}

// Heartbeat revalidates the license. Terminal server decisions stop the app
// immediately; connectivity/server failures are tolerated only for the
// configured offline grace period.
func Heartbeat() bool {
	if DevMode == "true" {
		return true
	}

	key := strings.TrimSpace(config.Env("LICENSE_KEY", ""))
	if key == "" {
		setVerificationState(false, "no_key", "LICENSE_KEY kosong", "")
		return false
	}

	machine, legacyMachine, err := machineFingerprints()
	if err != nil {
		setVerificationState(false, "machine_id_error", fmt.Sprintf("gagal membaca identitas instalasi: %v", err), "")
		return false
	}

	result := callVerifyAPI(key, machine, legacyMachine, true)
	now := time.Now()

	stateMu.Lock()
	defer stateMu.Unlock()
	if result.Valid {
		VerifyResult = true
		VerifyMessage = result.Message
		VerifyStatus = result.Status
		VerifyPackageType = result.PackageType
		lastHeartbeatOK = true
		lastHeartbeatAt = now
		heartbeatFailCnt = 0
		return true
	}

	heartbeatFailCnt++
	lastHeartbeatOK = false
	log.Printf("[license] Heartbeat gagal (x%d) status=%s: %s", heartbeatFailCnt, result.Status, result.Message)

	if isTerminalStatus(result.Status) {
		VerifyResult = false
		VerifyMessage = result.Message
		VerifyStatus = result.Status
		return false
	}

	grace := offlineGraceDuration()
	if lastHeartbeatAt.IsZero() || !now.Before(lastHeartbeatAt.Add(grace)) {
		VerifyResult = false
		VerifyStatus = StatusGraceExpired
		VerifyMessage = fmt.Sprintf("server lisensi tidak dapat diverifikasi selama %s: %s", grace, result.Message)
		return false
	}

	return true
}

func callVerifyAPI(key, machine, legacyMachine string, heartbeat bool) verificationResult {
	baseURL := licenseAPIBaseURL()
	nonce, err := newLicenseNonce()
	if err != nil {
		return verificationResult{Status: StatusInvalidRequest, Message: fmt.Sprintf("gagal membuat nonce lisensi: %v", err)}
	}
	body := verifyRequest{
		LicenseKey:        key,
		MachineHash:       machine,
		LegacyMachineHash: legacyMachine,
		Heartbeat:         heartbeat,
		Nonce:             nonce,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return verificationResult{Status: StatusInvalidRequest, Message: fmt.Sprintf("gagal menyusun request lisensi: %v", err)}
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/license/verify", bytes.NewReader(bodyJSON))
	if err != nil {
		return verificationResult{Status: StatusInvalidRequest, Message: fmt.Sprintf("gagal membuat request: %v", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	if secret := strings.TrimSpace(config.Env("LICENSE_API_SECRET", "")); secret != "" {
		req.Header.Set("X-License-Secret", secret)
	}

	resp, err := licenseHTTPClient.Do(req)
	if err != nil {
		return verificationResult{Status: StatusNetworkError, Message: fmt.Sprintf("server tidak terjangkau: %v", err)}
	}
	defer resp.Body.Close()

	var wrapper successWrapper
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return verificationResult{Status: statusFromHTTP(resp.StatusCode), Message: fmt.Sprintf("respons server tidak valid (HTTP %d)", resp.StatusCode)}
	}
	if wrapper.Data == nil {
		message := strings.TrimSpace(wrapper.Message)
		if message == "" {
			message = "server menolak verifikasi"
		}
		return verificationResult{Status: statusFromHTTP(resp.StatusCode), Message: message}
	}

	vr := wrapper.Data
	if err := verifyLicenseResponseSignature(key, machine, nonce, *vr); err != nil {
		return verificationResult{Status: StatusInvalidSignature, Message: err.Error()}
	}
	status := strings.TrimSpace(vr.Status)
	if status == "" {
		status = inferLegacyStatus(vr.Valid, vr.Message)
	}
	return verificationResult{
		Valid:          vr.Valid,
		Status:         status,
		Message:        vr.Message,
		PackageType:    vr.PackageType,
		ResetRemaining: vr.ResetRemaining,
		MachinesUsed:   vr.MachinesUsed,
		MachineMax:     vr.MachineMax,
	}
}

func licenseAPIBaseURL() string {
	if pinned := strings.TrimSpace(PinnedLicenseAPIURL); pinned != "" {
		return strings.TrimRight(pinned, "/")
	}
	return strings.TrimRight(config.Env("LICENSE_API_URL", "https://api.slaludiskon.com"), "/")
}

func newLicenseNonce() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

type licenseSignaturePayload struct {
	LicenseKeyHash string `json:"license_key_hash"`
	MachineHash    string `json:"machine_hash"`
	RequestNonce   string `json:"request_nonce"`
	SignedAt       int64  `json:"signed_at"`
	Valid          bool   `json:"valid"`
	Status         string `json:"status"`
	PackageType    string `json:"package_type"`
	ResetRemaining int    `json:"reset_remaining"`
	MachinesUsed   int    `json:"machines_used"`
	MachineMax     int    `json:"machine_max"`
	Message        string `json:"message"`
}

func verifyLicenseResponseSignature(licenseKey, machine, nonce string, response verifyResponse) error {
	configuredKey := strings.TrimSpace(PinnedLicenseSigningPublicKey)
	if configuredKey == "" {
		// Development and contract-test compatibility. Production builds should
		// embed PinnedLicenseSigningPublicKey with -ldflags.
		configuredKey = strings.TrimSpace(config.Env("LICENSE_RESPONSE_SIGNING_PUBLIC_KEY", ""))
	}
	if configuredKey == "" {
		return nil
	}
	publicKey, err := decodeEd25519PublicKey(configuredKey)
	if err != nil {
		return fmt.Errorf("public key signature lisensi tidak valid: %w", err)
	}
	if response.Signature == "" || response.RequestNonce != nonce {
		return fmt.Errorf("signature respons lisensi tidak ada atau nonce tidak cocok")
	}
	maxAge := time.Duration(config.EnvInt("LICENSE_SIGNATURE_MAX_AGE_SECONDS", 300)) * time.Second
	if maxAge < time.Second {
		maxAge = 300 * time.Second
	}
	now := time.Now()
	signedAt := time.Unix(response.SignedAt, 0)
	if signedAt.After(now.Add(30*time.Second)) || now.Sub(signedAt) > maxAge {
		return fmt.Errorf("signature respons lisensi sudah kedaluwarsa")
	}
	keyDigest := sha256.Sum256([]byte(strings.TrimSpace(licenseKey)))
	payload := licenseSignaturePayload{
		LicenseKeyHash: hex.EncodeToString(keyDigest[:]),
		MachineHash:    machine, RequestNonce: nonce, SignedAt: response.SignedAt,
		Valid: response.Valid, Status: response.Status, PackageType: response.PackageType,
		ResetRemaining: response.ResetRemaining, MachinesUsed: response.MachinesUsed,
		MachineMax: response.MachineMax, Message: response.Message,
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("gagal menyusun payload signature: %w", err)
	}
	signature, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(response.Signature))
	if err != nil {
		signature, err = base64.StdEncoding.DecodeString(strings.TrimSpace(response.Signature))
	}
	if err != nil || !ed25519.Verify(publicKey, canonical, signature) {
		return fmt.Errorf("signature respons lisensi tidak valid")
	}
	return nil
}

func decodeEd25519PublicKey(raw string) (ed25519.PublicKey, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(raw)
	}
	if err != nil {
		decoded, err = hex.DecodeString(raw)
	}
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key harus 32-byte base64 atau hex")
	}
	return ed25519.PublicKey(decoded), nil
}

func statusFromHTTP(statusCode int) string {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return StatusUnauthorized
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return StatusInvalidRequest
	case http.StatusTooManyRequests:
		return StatusRateLimited
	default:
		return StatusServerError
	}
}

func inferLegacyStatus(valid bool, message string) string {
	if valid {
		return StatusActive
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "revoked"), strings.Contains(lower, "suspend"):
		return StatusRevoked
	case strings.Contains(lower, "expired"):
		return StatusExpired
	case strings.Contains(lower, "mismatch"):
		return StatusMachineMismatch
	case strings.Contains(lower, "machine limit"), strings.Contains(lower, "limit reached"):
		return StatusMachineLimit
	case strings.Contains(lower, "not found"):
		return StatusNotFound
	default:
		return StatusServerError
	}
}

func isTerminalStatus(status string) bool {
	switch status {
	case StatusRevoked, StatusExpired, StatusMachineMismatch, StatusMachineLimit, StatusNotFound, StatusUnauthorized, StatusInvalidRequest, StatusInvalidSignature:
		return true
	default:
		return false
	}
}

func offlineGraceDuration() time.Duration {
	hours := config.EnvInt("LICENSE_OFFLINE_GRACE_HOURS", 24)
	if hours < 1 {
		hours = 1
	}
	return time.Duration(hours) * time.Hour
}

func setVerificationState(valid bool, status, message, packageType string) {
	stateMu.Lock()
	VerifyResult = valid
	VerifyStatus = status
	VerifyMessage = message
	VerifyPackageType = packageType
	stateMu.Unlock()
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "***"
}
