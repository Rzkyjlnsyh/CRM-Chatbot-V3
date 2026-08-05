package services

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"wa-assistant/backend/config"
	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	metaPixelIDKey       = "meta_pixel_id"
	metaAccessTokenKey   = "meta_capi_access_token"
	metaEnabledKey       = "meta_tracking_enabled"
	metaGraphVersionKey  = "meta_graph_version"
	metaTestEventCodeKey = "meta_test_event_code"
	metaDefaultVersion   = "v25.0"
	metaMaxAttempts      = 5
)

var metaGraphVersionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+$`)

type MetaTrackingConfig struct {
	Enabled       bool
	PixelID       string
	AccessToken   string
	GraphVersion  string
	TestEventCode string
}

type MetaTrackingConfigInput struct {
	Enabled          bool
	PixelID          string
	AccessToken      string
	GraphVersion     string
	TestEventCode    string
	ClearAccessToken bool
}

type MetaUserDataInput struct {
	Email      string
	Phone      string
	ExternalID string
	ClientIP   string
	UserAgent  string
	FBP        string
	FBC        string
}

type MetaEventInput struct {
	TenantID   uint
	EventID    string
	EventName  string
	EventTime  time.Time
	SourceURL  string
	UserData   MetaUserDataInput
	CustomData map[string]any
}

type MetaTrackingStats struct {
	Pending   int64                       `json:"pending"`
	Sent      int64                       `json:"sent"`
	Failed    int64                       `json:"failed"`
	LastEvent *models.MetaConversionEvent `json:"last_event,omitempty"`
}

func metaEncryptionKey() [32]byte {
	secret := strings.TrimSpace(config.Env("META_CAPI_SECRET_KEY", ""))
	if secret == "" {
		secret = strings.TrimSpace(config.EnvRequired("JWT_SECRET")) + "|meta-capi"
	}
	return sha256.Sum256([]byte(secret))
}

func encryptMetaSecret(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	key := metaEncryptionKey()
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
	return "v1:" + base64.RawStdEncoding.EncodeToString(payload), nil
}

func decryptMetaSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, "v1:") {
		return value, nil // kompatibilitas jika instalasi lama pernah menyimpan plaintext.
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, "v1:"))
	if err != nil {
		return "", err
	}
	key := metaEncryptionKey()
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("token CAPI terenkripsi tidak valid")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func GetMetaTrackingConfig() (MetaTrackingConfig, error) {
	encrypted := database.GetAppSetting(metaAccessTokenKey, "")
	token, err := decryptMetaSecret(encrypted)
	if err != nil {
		return MetaTrackingConfig{}, fmt.Errorf("gagal membaca token CAPI: %w", err)
	}
	version := strings.TrimSpace(database.GetAppSetting(metaGraphVersionKey, metaDefaultVersion))
	if !metaGraphVersionPattern.MatchString(version) {
		version = metaDefaultVersion
	}
	return MetaTrackingConfig{
		Enabled:       database.GetAppSetting(metaEnabledKey, "false") == "true",
		PixelID:       strings.TrimSpace(database.GetAppSetting(metaPixelIDKey, "")),
		AccessToken:   strings.TrimSpace(token),
		GraphVersion:  version,
		TestEventCode: strings.TrimSpace(database.GetAppSetting(metaTestEventCodeKey, "")),
	}, nil
}

func SaveMetaTrackingConfig(input MetaTrackingConfigInput) (MetaTrackingConfig, error) {
	current, err := GetMetaTrackingConfig()
	if err != nil {
		return MetaTrackingConfig{}, err
	}
	input.PixelID = strings.TrimSpace(input.PixelID)
	input.GraphVersion = strings.TrimSpace(input.GraphVersion)
	input.TestEventCode = strings.TrimSpace(input.TestEventCode)
	if input.GraphVersion == "" {
		input.GraphVersion = metaDefaultVersion
	}
	if input.PixelID != "" {
		for _, r := range input.PixelID {
			if r < '0' || r > '9' {
				return MetaTrackingConfig{}, errors.New("Pixel ID hanya boleh berisi angka")
			}
		}
	}
	if !metaGraphVersionPattern.MatchString(input.GraphVersion) {
		return MetaTrackingConfig{}, errors.New("versi Graph API harus seperti v25.0")
	}

	token := current.AccessToken
	if input.ClearAccessToken {
		token = ""
	}
	if strings.TrimSpace(input.AccessToken) != "" {
		token = strings.TrimSpace(input.AccessToken)
	}
	if input.Enabled && (input.PixelID == "" || token == "") {
		return MetaTrackingConfig{}, errors.New("Pixel ID dan token CAPI wajib diisi sebelum tracking diaktifkan")
	}

	encrypted, err := encryptMetaSecret(token)
	if err != nil {
		return MetaTrackingConfig{}, fmt.Errorf("gagal mengenkripsi token CAPI: %w", err)
	}
	enabledValue := "false"
	if input.Enabled {
		enabledValue = "true"
	}
	settings := []models.AppSetting{
		{Key: metaPixelIDKey, Value: input.PixelID},
		{Key: metaGraphVersionKey, Value: input.GraphVersion},
		{Key: metaTestEventCodeKey, Value: input.TestEventCode},
		{Key: metaAccessTokenKey, Value: encrypted},
		{Key: metaEnabledKey, Value: enabledValue},
	}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		for i := range settings {
			if err := tx.Save(&settings[i]).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return MetaTrackingConfig{}, fmt.Errorf("gagal menyimpan pengaturan Meta: %w", err)
	}
	return GetMetaTrackingConfig()
}

func PublicMetaPixelConfig() (enabled bool, pixelID string) {
	cfg, err := GetMetaTrackingConfig()
	if err != nil || !cfg.Enabled || cfg.PixelID == "" {
		return false, ""
	}
	return true, cfg.PixelID
}

func normalizeMetaPhone(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func metaHash(value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(value))))
	return hex.EncodeToString(sum[:])
}

func buildMetaUserData(input MetaUserDataInput) map[string]any {
	data := map[string]any{}
	if email := strings.ToLower(strings.TrimSpace(input.Email)); email != "" {
		data["em"] = []string{metaHash(email)}
	}
	if phone := normalizeMetaPhone(input.Phone); phone != "" {
		data["ph"] = []string{metaHash(phone)}
	}
	if externalID := strings.TrimSpace(input.ExternalID); externalID != "" {
		data["external_id"] = []string{metaHash(externalID)}
	}
	if value := strings.TrimSpace(input.ClientIP); value != "" {
		data["client_ip_address"] = value
	}
	if value := strings.TrimSpace(input.UserAgent); value != "" {
		data["client_user_agent"] = value
	}
	if value := strings.TrimSpace(input.FBP); value != "" {
		data["fbp"] = value
	}
	if value := strings.TrimSpace(input.FBC); value != "" {
		data["fbc"] = value
	}
	return data
}

func EnqueueMetaEvent(input MetaEventInput) error {
	cfg, err := GetMetaTrackingConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return nil
	}
	input.EventID = strings.TrimSpace(input.EventID)
	input.EventName = strings.TrimSpace(input.EventName)
	if input.EventID == "" || input.EventName == "" {
		return errors.New("event_id dan event_name Meta wajib diisi")
	}
	if len(input.EventID) > 100 || len(input.EventName) > 48 {
		return errors.New("event_id atau event_name Meta terlalu panjang")
	}
	if input.EventTime.IsZero() {
		input.EventTime = time.Now()
	}
	userData, err := json.Marshal(buildMetaUserData(input.UserData))
	if err != nil {
		return err
	}
	customData, err := json.Marshal(input.CustomData)
	if err != nil {
		return err
	}
	event := models.MetaConversionEvent{
		TenantID: input.TenantID, EventID: input.EventID, EventName: input.EventName,
		EventTime: input.EventTime, SourceURL: strings.TrimSpace(input.SourceURL),
		UserDataJSON: string(userData), CustomDataJSON: string(customData),
		Status: "pending", NextAttemptAt: time.Now(),
	}
	return database.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&event).Error
}

func metaEndpoint(cfg MetaTrackingConfig) string {
	return "https://graph.facebook.com/" + cfg.GraphVersion + "/" + url.PathEscape(cfg.PixelID) + "/events?access_token=" + url.QueryEscape(cfg.AccessToken)
}

func sendMetaConversionEvent(ctx context.Context, cfg MetaTrackingConfig, event models.MetaConversionEvent) (string, error) {
	var userData map[string]any
	if err := json.Unmarshal([]byte(event.UserDataJSON), &userData); err != nil {
		return "", fmt.Errorf("user_data event rusak: %w", err)
	}
	var customData map[string]any
	if event.CustomDataJSON != "" {
		if err := json.Unmarshal([]byte(event.CustomDataJSON), &customData); err != nil {
			return "", fmt.Errorf("custom_data event rusak: %w", err)
		}
	}
	serverEvent := map[string]any{
		"event_name":    event.EventName,
		"event_time":    event.EventTime.Unix(),
		"event_id":      event.EventID,
		"action_source": "website",
		"user_data":     userData,
	}
	if event.SourceURL != "" {
		serverEvent["event_source_url"] = event.SourceURL
	}
	if len(customData) > 0 {
		serverEvent["custom_data"] = customData
	}
	payload := map[string]any{"data": []any{serverEvent}}
	if cfg.TestEventCode != "" {
		payload["test_event_code"] = cfg.TestEventCode
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, metaEndpoint(cfg), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	responseText := string(responseBody)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseText, fmt.Errorf("Meta CAPI HTTP %d: %s", resp.StatusCode, responseText)
	}
	var result struct {
		EventsReceived int `json:"events_received"`
	}
	if json.Unmarshal(responseBody, &result) == nil && result.EventsReceived < 1 {
		return responseText, errors.New("Meta CAPI tidak mengonfirmasi penerimaan event")
	}
	return responseText, nil
}

func metaRetryDelay(attempt int) time.Duration {
	delays := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 4 * time.Hour}
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(delays) {
		return delays[len(delays)-1]
	}
	return delays[attempt-1]
}

func processMetaCAPIOutbox(ctx context.Context) {
	cfg, err := GetMetaTrackingConfig()
	if err != nil || !cfg.Enabled || cfg.PixelID == "" || cfg.AccessToken == "" {
		return
	}
	// Pulihkan event yang tertinggal jika proses mati setelah mengunci antrean.
	database.DB.Model(&models.MetaConversionEvent{}).
		Where("status = ? AND updated_at < ?", "sending", time.Now().Add(-2*time.Minute)).
		Updates(map[string]any{
			"status": "failed", "next_attempt_at": time.Now(),
			"last_error": "Pengiriman terputus saat server berhenti; dijadwalkan ulang.",
		})
	for i := 0; i < 20; i++ {
		var event models.MetaConversionEvent
		err := database.DB.Where("status IN ? AND attempts < ? AND next_attempt_at <= ?", []string{"pending", "failed"}, metaMaxAttempts, time.Now()).
			Order("id asc").First(&event).Error
		if err != nil {
			return
		}
		result := database.DB.Model(&models.MetaConversionEvent{}).
			Where("id = ? AND status IN ?", event.ID, []string{"pending", "failed"}).
			Updates(map[string]any{"status": "sending", "attempts": event.Attempts + 1})
		if result.Error != nil || result.RowsAffected == 0 {
			continue
		}
		event.Attempts++
		response, sendErr := sendMetaConversionEvent(ctx, cfg, event)
		if len(response) > 8000 {
			response = response[:8000]
		}
		if sendErr != nil {
			updates := map[string]any{
				"status": "failed", "last_error": sendErr.Error(), "meta_response": response,
				"next_attempt_at": time.Now().Add(metaRetryDelay(event.Attempts)),
			}
			if event.Attempts >= metaMaxAttempts {
				updates["user_data_json"] = ""
				updates["source_url"] = ""
			}
			database.DB.Model(&event).Updates(updates)
			log.Printf("[meta-capi] %s/%s gagal attempt %d: %v", event.EventName, event.EventID, event.Attempts, sendErr)
			continue
		}
		now := time.Now()
		database.DB.Model(&event).Updates(map[string]any{
			"status": "sent", "sent_at": &now, "last_error": "", "meta_response": response,
			"user_data_json": "", "source_url": "", // minimalkan retensi data request setelah event terkirim.
		})
	}
}

func StartMetaCAPIWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		Safe("processMetaCAPIOutbox", func() { processMetaCAPIOutbox(ctx) })
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				Safe("processMetaCAPIOutbox", func() { processMetaCAPIOutbox(ctx) })
			}
		}
	}()
}

func TestMetaCAPI(ctx context.Context, userData MetaUserDataInput) (string, error) {
	cfg, err := GetMetaTrackingConfig()
	if err != nil {
		return "", err
	}
	if !cfg.Enabled || cfg.PixelID == "" || cfg.AccessToken == "" {
		return "", errors.New("aktifkan tracking dan lengkapi Pixel ID serta token CAPI terlebih dahulu")
	}
	if cfg.TestEventCode == "" {
		return "", errors.New("isi Test Event Code agar tes tidak tercampur dengan data produksi")
	}
	userJSON, _ := json.Marshal(buildMetaUserData(userData))
	event := models.MetaConversionEvent{
		EventID: "test_" + fmt.Sprint(time.Now().UnixNano()), EventName: "PageView",
		EventTime: time.Now(), SourceURL: config.Env("APP_URL", ""),
		UserDataJSON: string(userJSON), CustomDataJSON: "{}",
	}
	return sendMetaConversionEvent(ctx, cfg, event)
}

func GetMetaTrackingStats() MetaTrackingStats {
	var stats MetaTrackingStats
	database.DB.Model(&models.MetaConversionEvent{}).Where("status IN ?", []string{"pending", "sending"}).Count(&stats.Pending)
	database.DB.Model(&models.MetaConversionEvent{}).Where("status = ?", "sent").Count(&stats.Sent)
	database.DB.Model(&models.MetaConversionEvent{}).Where("status = ?", "failed").Count(&stats.Failed)
	var last models.MetaConversionEvent
	if database.DB.Order("id desc").First(&last).Error == nil {
		last.UserDataJSON = ""
		last.CustomDataJSON = ""
		last.MetaResponse = ""
		stats.LastEvent = &last
	}
	return stats
}
