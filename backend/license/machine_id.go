package license

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"wa-assistant/backend/config"
)

func machineFingerprints() (current string, legacy string, err error) {
	path := config.Env("LICENSE_MACHINE_ID_PATH", "data/.license-machine-id")
	installationID, err := loadOrCreateInstallationID(path)
	if err != nil {
		return "", "", err
	}

	raw := "wa-assistant-license-v2|" + installationID + "|" + runtime.GOOS + "|" + runtime.GOARCH
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:16]), legacyMachineFingerprint(), nil
}

func loadOrCreateInstallationID(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("LICENSE_MACHINE_ID_PATH kosong")
	}

	if existing, err := os.ReadFile(path); err == nil {
		return validateInstallationID(string(existing))
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	id := hex.EncodeToString(random)

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			existing, readErr := os.ReadFile(path)
			if readErr != nil {
				return "", readErr
			}
			return validateInstallationID(string(existing))
		}
		return "", err
	}
	if _, err := file.WriteString(id + "\n"); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return id, nil
}

func validateInstallationID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 32 {
		return "", fmt.Errorf("installation ID tidak valid")
	}
	return id, nil
}

// legacyMachineFingerprint matches releases before the persisted installation
// ID. The LMS uses it only once to migrate an existing machine binding.
func legacyMachineFingerprint() string {
	host, _ := os.Hostname()
	username := "unknown"
	if currentUser, err := user.Current(); err == nil {
		username = currentUser.Username
	}
	raw := strings.ToLower(strings.TrimSpace(host)) + "|" +
		strings.ToLower(strings.TrimSpace(username)) + "|" +
		strings.ToLower(runtime.GOOS)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:16])
}
