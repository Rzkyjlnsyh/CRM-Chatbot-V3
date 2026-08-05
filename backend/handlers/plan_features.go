package handlers

// Semua fitur selalu diizinkan (internal company, tanpa paket langganan).
const planFeatureMessage = ""

// featAPI dipakai gating REST API (di edisi ini selalu diizinkan).
const featAPI = "api"

func tenantPlanAllows(tenantID uint, feature string) bool {
	return true
}

func agentPlanAllows(agentID uint, feature string) bool {
	return true
}
