package models

import "time"

// Flow = alur/menu bertingkat untuk satu agent (MVP: satu alur per agent). Strukturnya (pohon node)
// disimpan sebagai JSON di Structure. Pemicunya kata kunci (mis. "menu") yang, saat cocok, memulai
// alur dari node akar; balasan berikutnya dari kontak menavigasi menu sesuai pilihan.
type Flow struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	AgentID     uint      `gorm:"uniqueIndex;not null" json:"agent_id"`
	Enabled     bool      `gorm:"not null;default:false" json:"enabled"`
	Trigger     string    `gorm:"size:64" json:"trigger"`                            // kata kunci pemicu, mis "menu"
	MatchType   string    `gorm:"size:16;default:exact" json:"match_type"`           // exact, contains, prefix
	DisplayMode string    `gorm:"size:16;not null;default:auto" json:"display_mode"` // auto, text, buttons
	Structure   string    `gorm:"type:longtext" json:"structure"`                    // JSON: {root, nodes{id:{message,options[]}}}
	DelayMin    int       `gorm:"not null;default:2" json:"delay_min"`               // jeda minimum balasan alur (detik)
	DelayMax    int       `gorm:"not null;default:4" json:"delay_max"`               // jeda maksimum balasan alur (detik)
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// FlowSession menandai posisi kontak dalam alur (node yang sedang aktif). Satu sesi per (agent,sender);
// kedaluwarsa setelah tak ada aktivitas (lihat flowSessionTTL) agar tidak menjebak kontak selamanya.
type FlowSession struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AgentID   uint      `gorm:"uniqueIndex:idx_flow_session_ac;not null" json:"agent_id"`
	Sender    string    `gorm:"uniqueIndex:idx_flow_session_ac;size:32;not null" json:"sender"`
	NodeID    string    `gorm:"size:64" json:"node_id"`
	UpdatedAt time.Time `json:"updated_at"`
}
