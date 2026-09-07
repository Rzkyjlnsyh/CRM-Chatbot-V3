package services

import (
	"strings"
	"testing"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

func seedLincahOrder(t *testing.T, agentID uint, noOrder, resi, sender string) models.LincahOrder {
	t.Helper()
	o := models.LincahOrder{
		AgentID: agentID, Sender: sender, NoOrder: noOrder, Resi: resi,
		LincahOrderID: "lid-1", Status: "waiting", Courier: "jne",
		Name: "John Doe", Phone: "62856", ProductName: "Kaos", Weight: 1,
	}
	if err := database.DB.Create(&o).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	return o
}

func TestLincahParseWebhookShapes(t *testing.T) {
	shapes := []string{
		`{"no_order":"A1","resi":"R1","status":"In Transit"}`,
		`{"success":true,"data":{"no_order":"A1","resi":"R1","status":"Delivered","message":"Diterima"}}`,
		`{"order":{"no_order":"A1","resi":"R1","status":"Picked Up"}}`,
	}
	want := []string{"In Transit", "Delivered", "Picked Up"}
	for i, s := range shapes {
		ev, err := LincahParseWebhook([]byte(s))
		if err != nil {
			t.Fatalf("shape %d: %v", i, err)
		}
		if ev.NoOrder != "A1" || ev.Resi != "R1" || ev.Status != want[i] {
			t.Fatalf("shape %d salah: %+v", i, ev)
		}
	}
	if _, err := LincahParseWebhook([]byte(`{"foo":"bar"}`)); err == nil {
		t.Fatal("payload tanpa identitas harus error")
	}
	if _, err := LincahParseWebhook([]byte(`bukan json`)); err == nil {
		t.Fatal("payload bukan JSON harus error")
	}
}

func TestLincahWebhookUpdatesAndNotifies(t *testing.T) {
	setupLincahTestDB(t)
	if err := LincahSaveConfigFull(1, LincahConfigUpdate{
		Token: "tokentest", AutoNotify: true,
		NotifyTemplate: "Halo {{nama}}, paket {{resi}}: {{status}}{{message}}",
	}); err != nil {
		t.Fatal(err)
	}
	seedLincahOrder(t, 1, "NO-1", "RESI-1", "628561234567")

	sent := []string{}
	orig := lincahSendFn
	lincahSendFn = func(agentID uint, to, text string) error {
		sent = append(sent, to+"|"+text)
		return nil
	}
	t.Cleanup(func() { lincahSendFn = orig })

	raw := `{"no_order":"NO-1","resi":"RESI-1","status":"In Transit","message":"Sedang diantar kurir"}`
	ev, err := LincahProcessWebhook([]byte(raw))
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if ev.Status != "In Transit" {
		t.Fatalf("status salah: %+v", ev)
	}
	if len(sent) != 1 || !strings.Contains(sent[0], "RESI-1") || !strings.Contains(sent[0], "In Transit") || !strings.Contains(sent[0], "John") {
		t.Fatalf("notifikasi salah: %v", sent)
	}
	// Status SAMA dikirim lagi → TIDAK boleh spam (anti-duplikat).
	_, _ = LincahProcessWebhook([]byte(raw))
	if len(sent) != 1 {
		t.Fatalf("harusnya tidak kirim ulang status sama, dapat %d", len(sent))
	}
	// Status BARU → kirim lagi.
	_, err = LincahProcessWebhook([]byte(`{"no_order":"NO-1","resi":"RESI-1","status":"Delivered"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 {
		t.Fatalf("status baru harus dikirim, dapat %d", len(sent))
	}
	// DB ter-update.
	var o models.LincahOrder
	if err := database.DB.Where("no_order = ?", "NO-1").First(&o).Error; err != nil {
		t.Fatal(err)
	}
	if o.WebhookStatus != "Delivered" || o.NotifiedStatus != "Delivered" || o.LastNotifyAt == nil {
		t.Fatalf("state DB salah: %+v", o)
	}
}

func TestLincahWebhookNoNotifyWhenDisabled(t *testing.T) {
	setupLincahTestDB(t)
	// AutoNotify default false.
	seedLincahOrder(t, 2, "NO-2", "RESI-2", "62856")
	sent := 0
	orig := lincahSendFn
	lincahSendFn = func(agentID uint, to, text string) error { sent++; return nil }
	t.Cleanup(func() { lincahSendFn = orig })
	_, err := LincahProcessWebhook([]byte(`{"no_order":"NO-2","resi":"RESI-2","status":"Delivered"}`))
	if err != nil {
		t.Fatal(err)
	}
	if sent != 0 {
		t.Fatalf("tanpa AutoNotify tidak boleh kirim, dapat %d", sent)
	}
}

func TestLincahWebhookUnknownOrder(t *testing.T) {
	setupLincahTestDB(t)
	if _, err := LincahProcessWebhook([]byte(`{"no_order":"NO-X","resi":"RESI-X","status":"Delivered"}`)); err == nil {
		t.Fatal("pesanan tak dikenal harus error (best-effort, caller tetap 200)")
	}
}
