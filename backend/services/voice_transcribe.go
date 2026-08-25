package services

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// ── Transkripsi voice note (STT via OpenRouter, satu key yang sama) ─────────

// voiceTranscribeModel membaca setting "voice_model" (api_config DB), fallback
// ke model STT termurah di OpenRouter: gpt-4o-mini-transcribe.
func voiceTranscribeModel() string {
	return apiConfigFromDB("voice_model", "OPENROUTER_MODEL_STT", "openai/gpt-4o-mini-transcribe")
}

// TranscribeVoiceNote = ubah voice note jadi teks via OpenRouter STT.
func TranscribeVoiceNote(agentID uint, mimetype string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("audio kosong")
	}
	if len(data) > 25*1024*1024 {
		return "", fmt.Errorf("audio melebihi batas 25 MB")
	}

	model := voiceTranscribeModel()
	preset := activePreset()
	preset.Model = model
	if apiKeyForPreset(preset) == "" {
		return "", fmt.Errorf("API key OpenRouter kosong — transkripsi voice note nonaktif")
	}
	client := clientForPreset(preset)

	format := "ogg"
	switch strings.ToLower(mimetype) {
	case "audio/mpeg", "audio/mp3":
		format = "mp3"
	case "audio/wav", "audio/wave", "audio/x-wav":
		format = "wav"
	case "audio/mp4", "audio/x-m4a":
		format = "m4a"
	case "audio/webm":
		format = "webm"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	resp, err := client.CreateTranscription(ctx, openai.AudioRequest{
		Model:    model,
		FilePath: "voice." + format,
		Reader:   bytes.NewReader(data),
		Format:   openai.AudioResponseFormatJSON,
	})
	if err != nil {
		return "", fmt.Errorf("transkripsi gagal: %w", err)
	}
	text := strings.TrimSpace(resp.Text)
	if text == "" {
		return "", fmt.Errorf("transkripsi kosong")
	}
	return text, nil
}
