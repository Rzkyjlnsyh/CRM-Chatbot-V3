package models

import "time"

// LearningRun = satu sesi pembelajaran AI dari percakapan CS manusia.
type LearningRun struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	AgentID         uint       `gorm:"index;not null" json:"agent_id"`
	Status          string     `gorm:"size:16;index;default:pending" json:"status"` // pending, running, completed, failed
	SourceStartDate *time.Time `json:"source_start_date"`                           // awal rentang data yg dipelajari
	SourceEndDate   *time.Time `json:"source_end_date"`                             // akhir rentang data
	TotalChats      int        `gorm:"not null;default:0" json:"total_chats"`       // jumlah chat yg dianalisa
	HumanChats      int        `gorm:"not null;default:0" json:"human_chats"`       // chat dari CS manusia
	PatternCount    int        `gorm:"not null;default:0" json:"pattern_count"`     // pola yg berhasil diekstrak
	StyleProfile    string     `gorm:"type:text" json:"style_profile"`              // JSON profile gaya bahasa
	Summary         string     `gorm:"type:text" json:"summary"`                    // rekap: apa yg dipelajari & diterapkan
	Error           string     `gorm:"type:text" json:"error"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at"`
}

// LearningPattern = satu pola yg dipelajari dari CS manusia.
type LearningPattern struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	LearningRunID    uint       `gorm:"index;not null" json:"learning_run_id"`
	AgentID          uint       `gorm:"index;not null" json:"agent_id"`
	LabelID          string     `gorm:"size:64;index" json:"label_id"`         // label WhatsApp yg jadi konteks pola ("" = umum)
	LabelName        string     `gorm:"size:120" json:"label_name"`            // nama label (denormalisasi utk tampilan)
	PatternType      string     `gorm:"size:32;index" json:"pattern_type"`     // greeting, closing, objection_handling, upsell, tone, emoji_style, phrase, follow_up, label_handling, closing_path
	Source           string     `gorm:"size:16;index;default:human" json:"source"` // human = CS manusia; ai_success = chat AI yg closing
	TriggerContext   string     `gorm:"type:text" json:"trigger_context"`      // situasi yg memicu pola ini
	ResponseTemplate string     `gorm:"type:text" json:"response_template"`    // contoh balasan yg dipelajari
	EmojiSignature   string     `gorm:"size:120" json:"emoji_signature"`       // emoji yg sering dipakai
	Confidence       float64    `gorm:"not null;default:0" json:"confidence"`  // keyakinan AI akan efektivitas pola
	UsageCount       int        `gorm:"not null;default:0" json:"usage_count"` // seberapa sering dipakai
	ClosingImpact    float64    `gorm:"not null;default:0" json:"closing_impact"` // dampak thd closing rate (0-1)
	Status           string     `gorm:"size:16;index;default:suggested" json:"status"` // suggested, applied, rejected
	AppliedAt        *time.Time `json:"applied_at"`
	KnowledgeID      *uint      `gorm:"index" json:"knowledge_id"` // ID knowledge yg dibuat dari pola ini
	CreatedAt        time.Time  `json:"created_at"`
}

// LearningSnapshot = versi snapshot persona + knowledge untuk rollback.
type LearningSnapshot struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	AgentID        uint      `gorm:"index;not null" json:"agent_id"`
	LearningRunID  *uint     `gorm:"index" json:"learning_run_id"`              // opsional: terkait learning run mana
	SnapshotType   string    `gorm:"size:16;default:full" json:"snapshot_type"` // persona, knowledge, full
	Label          string    `gorm:"size:255" json:"label"`                     // label user-friendly
	DataJSON       string    `gorm:"type:text" json:"-"`                        // data terserialisasi
	PersonaAt      string    `gorm:"type:text" json:"persona_at"`               // isi persona saat snapshot
	KnowledgeCount int       `gorm:"not null;default:0" json:"knowledge_count"` // jumlah knowledge saat snapshot
	CreatedAt      time.Time `json:"created_at"`
}

// LearningConfig = pengaturan pembelajaran per agent.
type LearningConfig struct {
	ID                      uint      `gorm:"primaryKey" json:"id"`
	AgentID                 uint      `gorm:"uniqueIndex;not null" json:"agent_id"`
	Enabled                 bool      `gorm:"not null;default:false" json:"enabled"`
	AutoApply               bool      `gorm:"not null;default:false" json:"auto_apply"`
	MinConfidence           float64   `gorm:"not null;default:0.7" json:"min_confidence"`
	MinUsageCount           int       `gorm:"not null;default:3" json:"min_usage_count"`
	MaxPatternsPerRun       int       `gorm:"not null;default:10" json:"max_patterns_per_run"`
	PreserveManualKnowledge bool      `gorm:"not null;default:true" json:"preserve_manual_knowledge"`
	IncludeAIClosed         *bool     `json:"include_ai_closed"` // nil = true
	ScheduleEnabled         bool      `gorm:"not null;default:false" json:"schedule_enabled"`
	ScheduleCron            string    `gorm:"size:80" json:"schedule_cron"`             // "0 2 * * *"
	LookbackDays            int       `gorm:"not null;default:30" json:"lookback_days"` // analisa chat N hari terakhir
	UpdatedAt               time.Time `json:"updated_at"`
}

// PatternUsageLog = jejak pemakaian pola terpelajar di percakapan nyata
// (closed-loop: dampak closing diukur dari hasil, bukan estimasi AI).
type PatternUsageLog struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	PatternID uint       `gorm:"index;not null" json:"pattern_id"`
	AgentID   uint       `gorm:"index;not null" json:"agent_id"`
	Sender    string     `gorm:"index;size:32;not null" json:"sender"`
	Closed    bool       `gorm:"not null;default:false" json:"closed"`
	ClosedAt  *time.Time `json:"closed_at"`
	CreatedAt time.Time  `json:"created_at"`
}
