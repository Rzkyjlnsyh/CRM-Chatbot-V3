package models

import "time"

// Status broadcast
const (
	BroadcastPending         = "pending"
	BroadcastRunning         = "running"
	BroadcastDone            = "done"
	BroadcastInterrupted     = "interrupted"
	BroadcastWARestricted    = "wa_restricted"
	BroadcastResuming        = "resuming"
	BroadcastFailed          = "failed"
	BroadcastCancelRequested = "cancel_requested"
	BroadcastCancelled       = "cancelled"
)

// Broadcast = satu kampanye pesan massal milik sebuah agent.
type Broadcast struct {
	ID       uint `gorm:"primaryKey" json:"id"`
	TenantID uint `gorm:"index;not null" json:"tenant_id"`
	AgentID  uint `gorm:"index;not null" json:"agent_id"` // nomor utama (pertama di pool); dipakai untuk klaim & log
	// AgentIDs = daftar nomor (agent) yang ikut rotasi, JSON array mis. "[12,18,5]". Kosong =
	// broadcast satu-nomor (pakai AgentID saja). Penerima dibagi sticky antar nomor pool ini.
	AgentIDs           string `gorm:"type:text" json:"agent_ids,omitempty"`
	// QuarantineJSON = status karantina nomor selama rotasi (alasan, kode WA, cooldown).
	// Dipersist agar resume setelah wa_restricted tidak langsung memaksa nomor yang baru saja kena restriksi.
	QuarantineJSON     string `gorm:"type:text" json:"quarantine_json,omitempty"`
	Message            string `gorm:"type:text" json:"message"`
	ProductID          uint   `gorm:"index" json:"product_id,omitempty"`
	ProductButtonsJSON string `gorm:"type:text" json:"product_buttons_json,omitempty"`
	// TargetType menentukan makna kolom Number pada penerima:
	//   "number" (default) = nomor pribadi (@s.whatsapp.net)
	//   "group"            = JID grup (@g.us); pesan diposting ke dalam grup.
	TargetType       string     `gorm:"size:16;default:number;index" json:"target_type"`
	Status           string     `gorm:"size:16;default:pending;index" json:"status"` // pending, running, resuming, wa_restricted, done, interrupted, failed, cancel_requested, cancelled
	PauseReason      string     `gorm:"size:48" json:"pause_reason,omitempty"`
	PauseCode        int        `json:"pause_code,omitempty"`
	PausedAt         *time.Time `json:"paused_at,omitempty"`
	ConsentCategory  string     `gorm:"size:32;index" json:"consent_category"`
	ConsentSource    string     `gorm:"size:48" json:"consent_source"`
	RiskLevel        string     `gorm:"size:16;index" json:"risk_level"` // low, medium, high
	RiskReasons      string     `gorm:"type:text" json:"-"`
	RiskAcknowledged bool       `gorm:"not null;default:false" json:"risk_acknowledged"`
	OverrideReason   string     `gorm:"type:text" json:"override_reason,omitempty"`
	OverrideBy       *uint      `gorm:"index" json:"override_by,omitempty"`
	OverrideAt       *time.Time `json:"override_at,omitempty"`
	// Ritme kirim yang dipilih pengguna (detik).
	MinDelay int `json:"min_delay"`
	MaxDelay int `json:"max_delay"`
	// Istirahat panjang berkala.
	RestEvery    int `json:"rest_every"`
	RestDuration int `json:"rest_duration"`
	// Lampiran opsional.
	MediaType string `gorm:"size:16" json:"media_type"`
	MediaPath string `json:"-"`
	FileName  string `json:"file_name"`
	Mimetype  string `json:"mimetype"`
	// Kartu kontak (vCard) untuk broadcast "simpan kontak kami" (MediaType == "contact").
	ContactName   string    `gorm:"size:120" json:"contact_name"`
	ContactNumber string    `gorm:"size:32" json:"contact_number"`
	Total         int       `json:"total"`
	Sent          int       `json:"sent"`
	Failed        int       `json:"failed"`
	Skipped       int       `json:"skipped"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// BroadcastRecipient = satu penerima dalam sebuah broadcast.
type BroadcastRecipient struct {
	ID          uint `gorm:"primaryKey" json:"id"`
	BroadcastID uint `gorm:"index;not null" json:"broadcast_id"`
	// Number menyimpan nomor (broadcast biasa) ATAU JID grup "..@g.us" (broadcast grup).
	Number string `gorm:"size:64" json:"number"`
	Name   string `json:"name"`
	// AgentID = nomor (agent) yang mengirim penerima ini. Untuk rotasi nomor.
	AgentID uint       `gorm:"index" json:"agent_id"`
	Status  string     `gorm:"size:16;default:pending" json:"status"` // pending, sent, failed, skipped
	Error   string     `json:"error"`
	SentAt  *time.Time `json:"sent_at"`
}

// OptOut = kontak yang minta berhenti menerima pesan (balas STOP/BERHENTI).
type OptOut struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AgentID   uint      `gorm:"not null;uniqueIndex:idx_optout_agent_sender,priority:1" json:"agent_id"`
	Sender    string    `gorm:"not null;size:32;uniqueIndex:idx_optout_agent_sender,priority:2" json:"sender"`
	CreatedAt time.Time `json:"created_at"`
}

// ContactConsent menyimpan bukti izin per kontak dan kategori pesan.
// Consent tidak menghapus OptOut; kontak yang pernah meminta berhenti tetap harus dikeluarkan.
type ContactConsent struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	AgentID    uint       `gorm:"not null;uniqueIndex:idx_contact_consent,priority:1" json:"agent_id"`
	Number     string     `gorm:"size:32;not null;uniqueIndex:idx_contact_consent,priority:2" json:"number"`
	Category   string     `gorm:"size:32;not null;uniqueIndex:idx_contact_consent,priority:3" json:"category"`
	Source     string     `gorm:"size:48;not null" json:"source"`
	Note       string     `gorm:"type:text" json:"note"`
	GrantedAt  time.Time  `gorm:"index;not null" json:"granted_at"`
	RevokedAt  *time.Time `gorm:"index" json:"revoked_at,omitempty"`
	RecordedBy uint       `gorm:"index" json:"recorded_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
