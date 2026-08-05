#!/bin/bash
# ═══════════════════════════════════════════════════════════════
#  VALIDASI LENGKAP — SlaluDiskon AI Agent
#  Jalankan: bash validasi-lengkap.sh
# ═══════════════════════════════════════════════════════════════
set -e
cd "D:/jhe/chatloop-v1.2.0"

PASS=0
FAIL=0
check() { if [ "$1" = "true" ]; then PASS=$((PASS+1)); echo "  ✅ $2"; else FAIL=$((FAIL+1)); echo "  ❌ $2"; fi; }

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  VALIDASI LENGKAP — SlaluDiskon AI Agent                   ║"
check() {
  local result=$1
  local label=$2
  if [ "$result" = "0" ] || [ "$result" = "true" ]; then
    PASS=$((PASS+1))
    echo "  ✅ $label"
  else
    FAIL=$((FAIL+1))
    echo "  ❌ $label"
  fi
}

# ─── 1. BUILD & COMPILE ───────────────────────────────────────
echo ""
echo "━━━ 1. BUILD & COMPILE ━━━"
go build -ldflags "-X wa-assistant/backend/license.DevMode=true" -o /tmp/slalu-test.exe ./backend 2>&1 >/dev/null; check "$?" "Go build sukses"
go vet ./backend/... 2>&1 >/dev/null; check "$?" "Go vet clean"
cd frontend && npx tsc --noEmit --pretty 2>&1 >/dev/null; check "$?" "TypeScript compile clean"
cd "$OLDPWD"

# ─── 2. DIRECTIVE SYSTEM ──────────────────────────────────────
echo ""
echo "━━━ 2. DIRECTIVE SYSTEM ━━━"
check "$(grep -q 'handleSendMediaDirective' backend/handlers/agents.go && echo true)" "SEND_MEDIA handler"
check "$(grep -q 'sendMediaByLabels' backend/handlers/agents.go && echo true)" "SEND_MEDIA by label"
check "$(grep -q 'handleLabelDirective' backend/handlers/agents.go && echo true)" "LABEL directive"
check "$(grep -q 'handleBuatResiDirective' backend/handlers/agents.go && echo true)" "BUAT_RESI directive"
check "$(grep -q 'parseMediaLabels' backend/handlers/agents.go && echo true)" "Multi-label support"
check "$(grep -q 'sendSingleMedia' backend/handlers/agents.go && echo true)" "Single media send"

# ─── 3. SHIPPING / ONGKIR ─────────────────────────────────────
echo ""
echo "━━━ 3. ONGKIR & SHIPPING ━━━"
check "$(grep -q 'MENGANTAR_API_KEY' .env && echo true)" "Mengantar API key configured"
check "$(grep -q 'MENGANTAR_ORIGIN_AUTOFILL_ID' .env && echo true)" "Origin autofill ID"
check "$(grep -q 'maybeBuildShippingContext' backend/handlers/agents.go && echo true)" "Shipping context builder"
check "$(grep -q 'ONGKIR_REALTIME\|ONGKIR_AMBIGUOUS' backend/handlers/agents.go && echo true)" "ONGKIR prompt injection"
check "$(grep -q 'extractONGKIRBlock' backend/services/ai.go && echo true)" "ONGKIR block extractor"
check "$(grep -q 'jneUnsupported' backend/handlers/agents.go && echo true)" "JNE priority logic"
check "$(grep -q 'J&T.*REG.*JNE tidak tersedia' backend/handlers/agents.go && echo true)" "JT fallback label"
check "$(grep -q 'SHIPPING_TRANSFER_DISCOUNT' .env && echo true)" "Transfer discount config"
check "$(grep -q 'DISKON TRANSFER' backend/handlers/agents.go && echo true)" "Discount in prompt"

# ─── 4. CLOSING INSPECTOR ─────────────────────────────────────
echo ""
echo "━━━ 4. CLOSING INSPECTOR ━━━"
check "$(grep -q 'getContactClosedLabel' backend/handlers/agents.go && echo true)" "Closed label detection"
check "$(grep -q 'buildClosedContactReply' backend/handlers/agents.go && echo true)" "Closed reply handler"
check "$(grep -q 'ChatLabel' backend/models/label.go && echo true)" "Label model exists"
check "$(grep -q 'ClosingRecord' backend/models/models.go && echo true)" "Closing record model"

# ─── 5. LABEL TWO-WAY SYNC ────────────────────────────────────
echo ""
echo "━━━ 5. LABEL SYNC (WA ↔ Web) ━━━"
check "$(grep -q 'OnLabelEdit\|OnLabelAssoc' backend/handlers/labels_groups.go && echo true)" "WA → Web: real-time event handler"
check "$(grep -q 'ApplyLabel.*sender.*labelID' backend/services/wa.go && echo true)" "Web → WA: ApplyLabel method"
check "$(grep -q 'BuildLabelChat' backend/services/wa.go && echo true)" "Web → WA: appstate patch builder"
check "$(grep -q 'SendAppState' backend/services/wa.go && echo true)" "Web → WA: patch sender"
check "$(grep -q 'Sync ke WhatsApp asli' backend/handlers/agents.go && echo true)" "Web → WA: integrated in applyLabelToChat"

# ─── 6. INBOX PAGINATION ──────────────────────────────────────
echo ""
echo "━━━ 6. INBOX PAGINATION ━━━"
check "$(grep -q 'before_id' backend/handlers/features.go && echo true)" "Cursor param in API"
check "$(grep -q 'has_more' backend/handlers/features.go && echo true)" "has_more in response"
check "$(grep -q 'LIMIT 500' backend/handlers/features.go && echo true)" "Contacts limit 500"
check "$(grep -q 'useLoadOlderMessages' frontend/src/hooks.ts && echo true)" "Frontend: load older hook"
check "$(grep -q 'handleLoadOlder' frontend/src/components/InboxPanel.tsx && echo true)" "Frontend: load older handler"
check "$(grep -q 'Load Older Messages\|Pesan lama' frontend/src/components/InboxPanel.tsx && echo true)" "Frontend: load older button"

# ─── 7. API CONFIG (DASHBOARD) ────────────────────────────────
echo ""
echo "━━━ 7. API CONFIG (DASHBOARD) ━━━"
check "$(grep -q 'deepseek_api_key' backend/handlers/api_config.go && echo true)" "Backend: deepseek_api_key in config"
check "$(grep -q 'chat_provider' backend/handlers/api_config.go && echo true)" "Backend: chat_provider in config"
check "$(grep -q 'deepseekKey' frontend/src/pages/Dashboard.tsx && echo true)" "Frontend: DeepSeek key state"
check "$(grep -q 'chatProvider' frontend/src/pages/Dashboard.tsx && echo true)" "Frontend: Provider selector"
check "$(grep -q 'platform.deepseek.com' frontend/src/pages/Dashboard.tsx && echo true)" "Frontend: DeepSeek card"
check "$(grep -q 'deepseekBase' backend/services/ai.go && echo true)" "Backend: DeepSeek DB key lookup"

# ─── 8. MEDIA ASSETS ──────────────────────────────────────────
echo ""
echo "━━━ 8. MEDIA ASSETS ━━━"
check "$(grep -q 'Label.*string.*json:\"label\"' backend/models/media_asset.go && echo true)" "MediaAsset.Label field"
check "$(grep -q 'SortOrder' backend/models/media_asset.go && echo true)" "MediaAsset.SortOrder field"
check "$(grep -q 'label := strings.TrimSpace(c.PostForm(\"label\"))' backend/handlers/media_assets.go && echo true)" "Upload: label field"
check "$(grep -q 'ListMediaAssets\|UploadMediaAsset\|DeleteMediaAsset' backend/handlers/media_assets.go && echo true)" "CRUD endpoints"

# ─── 9. PERSONA FLOW ─────────────────────────────────────────
echo ""
echo "━━━ 9. PERSONA FLOW ━━━"
check "$(grep -q 'buildSystemPrompt' backend/services/ai.go && echo true)" "System prompt builder"
check "$(grep -q 'PERSONA KAMU' backend/services/ai.go && echo true)" "Persona injection"
check "$(grep -q 'trimPersonaForPrompt' backend/services/ai_advanced.go && echo true)" "Persona trimming"
check "$(grep -q 'factPriorityInstruction' backend/services/ai_advanced.go && echo true)" "Fact priority rules"

# ─── 10. AI MODEL ─────────────────────────────────────────────
echo ""
echo "━━━ 10. AI MODEL ━━━"
check "$(grep -q 'deepseek-direct\|deepseekBase' backend/services/ai.go && echo true)" "DeepSeek Direct preset"
check "$(grep -q 'DEEPSEEK_API_KEY\|deepseek_api_key' backend/services/ai.go && echo true)" "DeepSeek key fallback"
check "$(grep -q 'initAI\|activePreset' backend/services/ai.go && echo true)" "AI init + preset"
check "$(grep -q 'fallback.*deepseek\|fallback.*gemini' backend/services/ai.go && echo true)" "Model fallback chain"

# ─── 11. EMBEDDING & KNOWLEDGE ────────────────────────────────
echo ""
echo "━━━ 11. EMBEDDING & KNOWLEDGE ━━━"
check "$(grep -q 'searchKnowledge\|EmbeddingEnabled' backend/services/ai.go && echo true)" "Semantic search"
check "$(grep -q 'KnowledgeFor\|KBItem' backend/services/embedding.go && echo true)" "Knowledge cache"
check "$(grep -q 'IndexKnowledge\|BackfillEmbeddings' backend/services/embedding.go && echo true)" "Embedding indexer"

# ─── 12. VISION ───────────────────────────────────────────────
echo ""
echo "━━━ 12. VISION (BUKTI TRANSFER) ━━━"
check "$(grep -q 'AnalyzeCustomerImage' backend/services/vision.go && echo true)" "Vision analyzer"
check "$(grep -q 'visionProductCatalog' backend/services/vision.go && echo true)" "Product matching"
check "$(grep -q 'ImageAnalysis' backend/models/models.go && echo true)" "Image analysis model"

# ─── 13. TESTS ────────────────────────────────────────────────
echo ""
echo "━━━ 13. UNIT TESTS ━━━"
JWT_SECRET=test_secret_32chars_minimum_for_jwt_1234567890 go test ./backend/handlers/... -count=1 -timeout 60s 2>&1 >/dev/null; check "$?" "Handler tests pass"
JWT_SECRET=test_secret_32chars_minimum_for_jwt_1234567890 go test ./backend/services/... -count=1 -timeout 60s 2>&1 >/dev/null; check "$?" "Service tests pass"

# ─── SUMMARY ──────────────────────────────────────────────────
echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  HASIL VALIDASI                                              ║"
echo "╠══════════════════════════════════════════════════════════════╣"
echo "║  ✅ PASS : $PASS                                                  ║"
echo "║  ❌ FAIL : $FAIL                                                   ║"
echo "╚══════════════════════════════════════════════════════════════╝"
TOTAL=$((PASS + FAIL))
PCT=$((PASS * 100 / TOTAL))
echo "  Score: $PASS/$TOTAL ($PCT%)"
if [ "$FAIL" -eq 0 ]; then
  echo "  Status: SEMUA VALID — 100% ready!"
else
  echo "  Status: Ada $FAIL item yang perlu diperbaiki"
fi

rm -f /tmp/slalu-test.exe
