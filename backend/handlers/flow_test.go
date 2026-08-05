package handlers

import (
	"strings"
	"testing"
)

func validTestFlow() flowStructure {
	return flowStructure{
		Root: "start",
		Nodes: map[string]flowNode{
			"start": {
				Message: "Pilih bantuan",
				Options: []flowOption{
					{Key: "1", Label: "Produk", Action: "goto", Target: "products"},
					{Key: "2", Label: "CS", Action: "handoff", Reply: "Diteruskan ke CS"},
				},
			},
			"products": {
				Message: "Daftar produk",
				Options: []flowOption{{Key: "1", Label: "Katalog", Action: "reply_menu", Reply: "Berikut katalog"}},
			},
		},
	}
}

func TestValidateFlowStructure(t *testing.T) {
	if err := validateFlowStructure(validTestFlow()); err != nil {
		t.Fatalf("alur valid ditolak: %v", err)
	}

	duplicate := validTestFlow()
	node := duplicate.Nodes["start"]
	node.Options = append(node.Options, flowOption{Key: "1", Action: "reply", Reply: "duplikat"})
	duplicate.Nodes["start"] = node
	if err := validateFlowStructure(duplicate); err == nil || !strings.Contains(err.Error(), "dua kali") {
		t.Fatalf("kode pilihan ganda harus ditolak, dapat %v", err)
	}

	brokenTarget := validTestFlow()
	node = brokenTarget.Nodes["start"]
	node.Options[0].Target = "missing"
	brokenTarget.Nodes["start"] = node
	if err := validateFlowStructure(brokenTarget); err == nil || !strings.Contains(err.Error(), "tidak ditemukan") {
		t.Fatalf("target hilang harus ditolak, dapat %v", err)
	}
}

func TestRenderFlowNodeMessageBuildsChoices(t *testing.T) {
	flow := validTestFlow()
	message := renderFlowNodeMessage("start", flow.Nodes["start"], flow.Root)
	for _, expected := range []string{"Pilih bantuan", "1. Produk", "2. CS", "keluar"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("preview tidak memuat %q: %s", expected, message)
		}
	}
}

func TestRenderFlowNodeResultUsesAdaptiveButtons(t *testing.T) {
	flow := validTestFlow()
	node := flow.Nodes["start"]
	result := renderFlowNodeResult("start", node, flow.Root, "auto", "")
	if len(result.buttons) != 2 || result.buttons[0].ID != "flow:1" {
		t.Fatalf("mode otomatis tidak membuat tombol: %#v", result.buttons)
	}
	if !strings.Contains(result.fallback, "1. Produk") {
		t.Fatalf("fallback menu angka tidak tersedia: %q", result.fallback)
	}

	node.Options = append(node.Options,
		flowOption{Key: "3", Label: "FAQ", Action: "reply", Reply: "FAQ"},
		flowOption{Key: "4", Label: "Lokasi", Action: "reply", Reply: "Lokasi"},
	)
	result = renderFlowNodeResult("start", node, flow.Root, "auto", "")
	if len(result.buttons) != 0 || !strings.Contains(result.reply, "4. Lokasi") {
		t.Fatalf("empat pilihan seharusnya memakai menu angka: %#v %q", result.buttons, result.reply)
	}
}

func TestRenderFlowNodeResultTextModeNeverUsesButtons(t *testing.T) {
	flow := validTestFlow()
	result := renderFlowNodeResult("start", flow.Nodes["start"], flow.Root, "text", "")
	if len(result.buttons) != 0 || !strings.Contains(result.reply, "1. Produk") {
		t.Fatalf("mode teks tidak dirender sebagai menu angka: %#v %q", result.buttons, result.reply)
	}
}

func TestParentFlowNode(t *testing.T) {
	parent, ok := parentFlowNode(validTestFlow(), "products")
	if !ok || parent != "start" {
		t.Fatalf("parent submenu = %q, %v", parent, ok)
	}
}

func TestLooksLikeFlowCodeDoesNotCaptureNaturalReply(t *testing.T) {
	for _, natural := range []string{"Berfungsi baik sih kak cuma ada cacat dikit", "iya", "baik"} {
		if looksLikeFlowCode(natural, "") {
			t.Fatalf("kalimat lanjutan AI tidak boleh dianggap kode menu: %q", natural)
		}
	}
	for _, input := range []string{"1", "A", "99"} {
		if !looksLikeFlowCode(input, "") {
			t.Fatalf("kode menu singkat tidak dikenali: %q", input)
		}
	}
	if !looksLikeFlowCode("apa pun", "flow:1") {
		t.Fatal("action ID tombol harus selalu dianggap input menu")
	}
}

func TestContainsTriggerUsesWordBoundary(t *testing.T) {
	if matchTrigger("Berfungsi baik kak", "fungsi", "contains") {
		t.Fatal("pemicu contains tidak boleh cocok di tengah kata")
	}
	if !matchTrigger("Tolong buka menu ya", "menu", "contains") {
		t.Fatal("pemicu contains harus cocok sebagai kata utuh")
	}
}
