package license

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"wa-assistant/backend/config"
)

type resetRequest struct {
	LicenseKey  string `json:"license_key"`
	MachineHash string `json:"machine_hash,omitempty"`
}

type resetWrapper struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Reset clears the current machine binding and consumes one customer reset.
// It is exposed through the `license-reset` CLI command, not through HTTP UI.
func Reset() error {
	if DevMode == "true" {
		return nil
	}
	key := strings.TrimSpace(config.Env("LICENSE_KEY", ""))
	if key == "" {
		return fmt.Errorf("LICENSE_KEY kosong")
	}
	if !config.EnvBool("LICENSE_PUBLIC_RESET_ENABLED", false) {
		return fmt.Errorf("reset perangkat dilakukan dari dashboard member LMS; endpoint reset publik sedang dinonaktifkan")
	}
	machine, _, err := machineFingerprints()
	if err != nil {
		return fmt.Errorf("gagal menyiapkan identitas instalasi: %w", err)
	}
	body, err := json.Marshal(resetRequest{LicenseKey: key, MachineHash: machine})
	if err != nil {
		return err
	}
	baseURL := licenseAPIBaseURL()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/license/reset", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if secret := strings.TrimSpace(config.Env("LICENSE_API_SECRET", "")); secret != "" {
		req.Header.Set("X-License-Secret", secret)
	}
	resp, err := licenseHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("server lisensi tidak terjangkau: %w", err)
	}
	defer resp.Body.Close()
	var wrapper resetWrapper
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return fmt.Errorf("respons reset tidak valid (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !wrapper.Success {
		if wrapper.Message == "" {
			wrapper.Message = "reset lisensi ditolak"
		}
		return fmt.Errorf("%s", wrapper.Message)
	}
	return nil
}
