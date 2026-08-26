package handlers

import (
	"testing"

	"wa-assistant/backend/models"
	"wa-assistant/backend/services"
)

// defsTahap = definisi pipeline standar untuk menguji guardrail penerapan AI.
var defsTahap = map[string]models.LeadStageDef{
	"unqualified": {Key: "unqualified", Rank: 0, MinConfidence: 0.9},
	"new":         {Key: "new", Rank: 1, MinConfidence: 0.72},
	"cold":        {Key: "cold", Rank: 2, MinConfidence: 0.72},
	"warm":        {Key: "warm", Rank: 3, MinConfidence: 0.72},
	"hot":         {Key: "hot", Rank: 4, MinConfidence: 0.82},
	"customer":    {Key: "customer", Rank: 5, IsClosing: true},
}

func TestCanApplyAILeadStage(t *testing.T) {
	if canApplyAILeadStage(models.Contact{LeadStage: "new", LeadStageLocked: true}, services.CRMLeadAssessment{Stage: "hot", Confidence: .99}, defsTahap) {
		t.Fatal("status manual terkunci tidak boleh ditimpa AI")
	}
	if canApplyAILeadStage(models.Contact{LeadStage: "customer"}, services.CRMLeadAssessment{Stage: "hot", Confidence: .99}, defsTahap) {
		t.Fatal("customer tidak boleh diturunkan AI")
	}
	if canApplyAILeadStage(models.Contact{LeadStage: "new"}, services.CRMLeadAssessment{Stage: "hot", Confidence: .7}, defsTahap) {
		t.Fatal("hot berkeyakinan rendah tidak boleh diterapkan")
	}
	if !canApplyAILeadStage(models.Contact{LeadStage: "new"}, services.CRMLeadAssessment{Stage: "warm", Confidence: .86}, defsTahap) {
		t.Fatal("warm berkeyakinan tinggi harus dapat diterapkan")
	}
	if canApplyAILeadStage(models.Contact{LeadStage: "hot", LeadStageSource: "activity"}, services.CRMLeadAssessment{Stage: "cold", Confidence: .99}, defsTahap) {
		t.Fatal("AI tidak boleh menurunkan sinyal aktivitas eksplisit")
	}
	// AI tidak pernah menetapkan tahap closing, seberapa pun yakinnya.
	if canApplyAILeadStage(models.Contact{LeadStage: "hot"}, services.CRMLeadAssessment{Stage: "customer", Confidence: .99}, defsTahap) {
		t.Fatal("AI tidak boleh menetapkan tahap closing/customer")
	}
	// Pengecualian monotonic: turun ke tahap terendah (bucket negatif) diizinkan dengan threshold tinggi.
	if canApplyAILeadStage(models.Contact{LeadStage: "warm"}, services.CRMLeadAssessment{Stage: "unqualified", Confidence: .85}, defsTahap) {
		t.Fatal("unqualified di bawah ambang 0.9 tidak boleh diterapkan")
	}
	if !canApplyAILeadStage(models.Contact{LeadStage: "warm"}, services.CRMLeadAssessment{Stage: "unqualified", Confidence: .95}, defsTahap) {
		t.Fatal("unqualified berkeyakinan tinggi boleh diterapkan (deteksi spam)")
	}
	// Tahap custom user ikut dihormati.
	defsCustom := map[string]models.LeadStageDef{
		"new":     {Key: "new", Rank: 0, MinConfidence: 0.72},
		"deal":    {Key: "deal", Rank: 1, MinConfidence: 0.9},
		"closing": {Key: "closing", Rank: 2, IsClosing: true},
	}
	if canApplyAILeadStage(models.Contact{LeadStage: "new"}, services.CRMLeadAssessment{Stage: "closing", Confidence: .99}, defsCustom) {
		t.Fatal("AI tidak boleh menetapkan tahap closing custom")
	}
	if canApplyAILeadStage(models.Contact{LeadStage: "new"}, services.CRMLeadAssessment{Stage: "deal", Confidence: .85}, defsCustom) {
		t.Fatal("tahap custom deal butuh keyakinan >= 0.9 (ambang user)")
	}
	if !canApplyAILeadStage(models.Contact{LeadStage: "new"}, services.CRMLeadAssessment{Stage: "deal", Confidence: .93}, defsCustom) {
		t.Fatal("tahap custom deal dengan keyakinan cukup harus diterapkan")
	}
}
