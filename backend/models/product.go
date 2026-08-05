package models

import "time"

type ProductDetail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Product = satu item katalog yang bisa dikirim ke pelanggan oleh AI atau manual dari dashboard.
type Product struct {
	ID                     uint      `gorm:"primaryKey" json:"id"`
	TenantID               uint      `gorm:"index;not null" json:"tenant_id"`
	AgentID                uint      `gorm:"index;not null" json:"agent_id"`
	Name                   string    `gorm:"size:255;not null" json:"name"`
	ProductType            string    `gorm:"size:32;not null;default:physical" json:"product_type"`
	Price                  string    `gorm:"size:64" json:"price"` // fleksibel: "Rp 75.000", "$5", dll
	Description            string    `gorm:"type:text" json:"description"`
	DetailsJSON            string    `gorm:"type:text" json:"details_json"`      // atribut fleksibel fisik/digital/jasa/dll
	Knowledge              string    `gorm:"type:text" json:"knowledge"`         // fakta/FAQ internal khusus produk
	AISalesGuidance        string    `gorm:"type:text" json:"ai_sales_guidance"` // arahan follow-up/penjualan khusus produk
	ImagePath              string    `gorm:"size:512" json:"image_path"`         // path di filesystem server
	ImageMime              string    `gorm:"size:64" json:"image_mime"`
	ImageURL               string    `gorm:"-" json:"image_url,omitempty"`
	ButtonsJSON            string    `gorm:"type:text" json:"buttons_json"`
	CheckoutStepsJSON      string    `gorm:"type:text" json:"checkout_steps_json"`
	CheckoutHandoff        bool      `gorm:"not null;default:true" json:"checkout_handoff"`
	CheckoutSuccessMessage string    `gorm:"type:text" json:"checkout_success_message"`
	// Embedding dipakai hybrid retrieval katalog (parafrase / tanpa kata persis di nama).
	Embedding      string `gorm:"type:longtext" json:"-"`
	EmbeddingModel string `gorm:"size:80" json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ProductCheckoutSession menyimpan progres checkout per kontak agar tahan restart.
type ProductCheckoutSession struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TenantID  uint      `gorm:"index;not null" json:"tenant_id"`
	AgentID   uint      `gorm:"uniqueIndex:idx_checkout_agent_sender;not null" json:"agent_id"`
	Sender    string    `gorm:"uniqueIndex:idx_checkout_agent_sender;size:32;not null" json:"sender"`
	ProductID uint      `gorm:"index;not null" json:"product_id"`
	StepIndex int       `gorm:"not null;default:0" json:"step_index"`
	DataJSON  string    `gorm:"type:text" json:"data_json"`
	Status    string    `gorm:"size:24;index;not null;default:collecting" json:"status"`
	ExpiresAt time.Time `gorm:"index" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProductOrder adalah hasil checkout yang sudah dikonfirmasi pelanggan.
type ProductOrder struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TenantID  uint      `gorm:"index;not null" json:"tenant_id"`
	AgentID   uint      `gorm:"index;not null" json:"agent_id"`
	ProductID uint      `gorm:"index;not null" json:"product_id"`
	Sender    string    `gorm:"index;size:32;not null" json:"sender"`
	OrderCode string    `gorm:"uniqueIndex;size:32;not null" json:"order_code"`
	Status    string    `gorm:"size:24;index;not null;default:pending_cs" json:"status"`
	DataJSON  string    `gorm:"type:text" json:"data_json"`
	Summary   string    `gorm:"type:text" json:"summary"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Product   Product   `gorm:"foreignKey:ProductID" json:"product"`
}
