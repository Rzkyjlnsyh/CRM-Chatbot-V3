package models

import "time"

// ShippingOrder merepresentasikan pesanan pengiriman yang dibuat via Mengantar API.
// Tabel: shipping_orders
type ShippingOrder struct {
	ID                       uint       `gorm:"primaryKey" json:"id"`
	AgentID                  uint       `gorm:"index" json:"agent_id"`
	TenantID                 uint       `gorm:"index;default:1" json:"tenant_id"`
	ChatSender               string     `gorm:"size:20;index" json:"chat_sender"`          // nomor customer dari WhatsApp
	MengantarOrderID         string     `gorm:"size:30;uniqueIndex" json:"mengantar_order_id"` // ORDER_ID dari Mengantar
	MengantarBatchID         string     `gorm:"size:30" json:"mengantar_batch_id"`         // batch_id dari Mengantar
	CnoteNo                  string     `gorm:"size:30;index" json:"cnote_no"`             // nomor resi
	Courier                  string     `gorm:"size:10" json:"courier"`                    // JNE / JT
	CustomerName             string     `gorm:"size:100" json:"customer_name"`
	CustomerAddress          string     `gorm:"size:500" json:"customer_address"`
	CustomerPhone            string     `gorm:"size:15" json:"customer_phone"`
	DestinationAddressDataID string     `gorm:"size:30" json:"destination_address_data_id"` // _id dari address/search
	OriginAddressID          string     `gorm:"size:30" json:"origin_address_id"`          // _id saved address (pickup)
	OriginAutofillID         string     `gorm:"size:30" json:"origin_autofill_id"`         // PICKUP_AUTOFILL
	WeightGram               int        `json:"weight_gram"`                              // berat dalam gram
	Quantity                 int        `json:"quantity"`
	ParcelContent            string     `gorm:"size:200" json:"parcel_content"`
	GoodsValue               int        `json:"goods_value"`                              // nilai barang
	CodAmount                int        `json:"cod_amount"`                               // nilai COD
	ShippingCost             int        `json:"shipping_cost"`                            // ongkir (harga normal)
	ShippingCostDiscounted   int        `json:"shipping_cost_discounted"`                 // ongkir setelah diskon
	Discount                 int        `json:"discount"`                                 // jumlah diskon
	EstimatedDelivery        string     `gorm:"size:50" json:"estimated_delivery"`        // estimasi pengiriman
	Status                   string     `gorm:"size:30;index;default:active" json:"status"` // active/ON_DELIVERY/DELIVERED/RTS/dll
	StatusCategory           string     `gorm:"size:30" json:"status_category"`
	LastTrackingDate         string     `gorm:"size:50" json:"last_tracking_date"`        // update tracking terakhir
	LastTrackingDesc         string     `gorm:"size:500" json:"last_tracking_desc"`
	RawHistory               string     `gorm:"type:text" json:"raw_history,omitempty"`   // JSON history dari Mengantar
	IsPaid                   bool       `gorm:"default:true" json:"is_paid"`
	NotifiedDelivered        bool       `gorm:"default:false" json:"notified_delivered"`   // sudah dikirim notif delivered?
	NotifiedPickedUp         bool       `gorm:"default:false" json:"notified_picked_up"`   // sudah dikirim notif pickup?
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

// TableName custom
func (ShippingOrder) TableName() string {
	return "shipping_orders"
}
