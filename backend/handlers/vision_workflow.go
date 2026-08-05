package handlers

import (
	"fmt"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"
)

type visionWorkflowContext struct {
	Kind        string
	Instruction string
}

func activeVisionWorkflow(agentID uint, sender string) visionWorkflowContext {
	var checkout models.ProductCheckoutSession
	if database.DB.Where("agent_id = ? AND sender = ? AND status = ?", agentID, sender, "collecting").First(&checkout).Error == nil && time.Now().Before(checkout.ExpiresAt) {
		var product models.Product
		if database.DB.Where("tenant_id = ?", checkout.TenantID).First(&product, checkout.ProductID).Error == nil {
			steps := parseCheckoutSteps(product)
			if checkout.StepIndex >= 0 && checkout.StepIndex < len(steps) {
				step := steps[checkout.StepIndex]
				return visionWorkflowContext{Kind: "checkout", Instruction: visionFieldInstruction("checkout produk "+product.Name, step.Label, step.Type, step.Options)}
			}
		}
	}
	var session models.AIFormSession
	if database.DB.Where("agent_id = ? AND sender = ? AND status = ?", agentID, sender, "collecting").First(&session).Error == nil && time.Now().Before(session.ExpiresAt) {
		var form models.AIForm
		if database.DB.Where("agent_id = ?", agentID).First(&form, session.FormID).Error == nil {
			steps := parseAIFormSteps(form)
			if session.StepIndex >= 0 && session.StepIndex < len(steps) {
				step := steps[session.StepIndex]
				return visionWorkflowContext{Kind: "form", Instruction: visionFieldInstruction("Form Layanan "+form.Name, step.Label, step.Type, step.Options)}
			}
		}
	}
	return visionWorkflowContext{}
}

func visionFieldInstruction(flowName, question, answerType string, options []string) string {
	instruction := fmt.Sprintf("Pelanggan sedang mengisi %s. Pertanyaan aktif: %s. Ambil answer hanya untuk pertanyaan ini dengan jenis %s.", flowName, strings.TrimSpace(question), answerType)
	if len(options) > 0 {
		instruction += " answer wajib persis salah satu pilihan: " + strings.Join(options, ", ") + "."
	}
	return instruction
}

func visionProductButtons(agentID, productID uint) []services.ReplyButton {
	if productID == 0 {
		return nil
	}
	var product models.Product
	if database.DB.Where("agent_id = ?", agentID).First(&product, productID).Error != nil {
		return nil
	}
	buttons := make([]services.ReplyButton, 0, 3)
	for _, button := range parseProductButtons(product) {
		if len(buttons) >= 3 {
			break
		}
		buttons = append(buttons, services.ReplyButton{
			ID: fmt.Sprintf("product:%d:%s", product.ID, button.Key), Text: productButtonDisplayText(button),
		})
	}
	return buttons
}
