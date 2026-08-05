package handlers

import (
	"fmt"
	"testing"

	"wa-assistant/backend/models"
)

func TestHistoryWithinContextBudgetIsNotLimitedToTwentyMessages(t *testing.T) {
	newestFirst := make([]models.ChatHistory, 35)
	for i := range newestFirst {
		newestFirst[i] = models.ChatHistory{ID: uint(35 - i), Message: fmt.Sprintf("pesan-%02d", 35-i), Reply: "baik"}
	}

	got := historyWithinContextBudget(newestFirst, 24000)
	if len(got) != 35 {
		t.Fatalf("jumlah konteks = %d, ingin 35", len(got))
	}
	if got[0].ID != 1 || got[len(got)-1].ID != 35 {
		t.Fatalf("urutan konteks tidak kronologis: pertama=%d terakhir=%d", got[0].ID, got[len(got)-1].ID)
	}
}

func TestHistoryWithinContextBudgetKeepsNewestConversation(t *testing.T) {
	newestFirst := []models.ChatHistory{
		{ID: 4, Message: "4444", Reply: "4444"},
		{ID: 3, Message: "3333", Reply: "3333"},
		{ID: 2, Message: "2222", Reply: "2222"},
		{ID: 1, Message: "1111", Reply: "1111"},
	}

	got := historyWithinContextBudget(newestFirst, 17)
	if len(got) != 2 || got[0].ID != 3 || got[1].ID != 4 {
		t.Fatalf("konteks terbaru tidak terpilih dengan benar: %+v", got)
	}
}
