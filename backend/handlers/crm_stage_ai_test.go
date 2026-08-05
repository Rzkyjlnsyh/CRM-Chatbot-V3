package handlers

import (
	"testing"

	"wa-assistant/backend/models"
	"wa-assistant/backend/services"
)

func TestCanApplyAILeadStage(t *testing.T) {
	if canApplyAILeadStage(models.Contact{LeadStage: "new", LeadStageLocked: true}, services.CRMLeadAssessment{Stage: "hot", Confidence: .99}) {
		t.Fatal("status manual terkunci tidak boleh ditimpa AI")
	}
	if canApplyAILeadStage(models.Contact{LeadStage: "customer"}, services.CRMLeadAssessment{Stage: "hot", Confidence: .99}) {
		t.Fatal("customer tidak boleh diturunkan AI")
	}
	if canApplyAILeadStage(models.Contact{LeadStage: "new"}, services.CRMLeadAssessment{Stage: "hot", Confidence: .7}) {
		t.Fatal("hot berkeyakinan rendah tidak boleh diterapkan")
	}
	if !canApplyAILeadStage(models.Contact{LeadStage: "new"}, services.CRMLeadAssessment{Stage: "warm", Confidence: .86}) {
		t.Fatal("warm berkeyakinan tinggi harus dapat diterapkan")
	}
	if canApplyAILeadStage(models.Contact{LeadStage: "hot", LeadStageSource: "activity"}, services.CRMLeadAssessment{Stage: "cold", Confidence: .99}) {
		t.Fatal("AI tidak boleh menurunkan sinyal aktivitas eksplisit")
	}
}
