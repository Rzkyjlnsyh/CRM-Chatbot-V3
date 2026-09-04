# 🚀 Implementation Plan: Merge chatloop-1.6-1.7 → chatloop-v1.2.0-jhe

## Tujuan
Mengambil semua update/bugfix dari versi seller terbaru (1.6-1.7) dan meng-integrasikannya ke codebase Qahira (JHE), **tanpa menghilangkan satu pun fitur kustom JHE**, sambil meningkatkan kualitas secara keseluruhan.

---

## 🔑 Prinsip Merge

1. **JHE-first**: JHE adalah base. Kita tambahkan dari 1.6-1.7, bukan sebaliknya.
2. **Additive, bukan replace**: Fungsi baru 1.6-1.7 ditambahkan. Fungsi existing JHE tetap.
3. **Conflict resolution**: Jika ada overlap, ambil versi yang lebih lengkap + preserve logika kustom JHE.
4. **Test setelah setiap phase**: Build harus berhasil sebelum lanjut.

---

## 📊 Scope of Changes (Ringkasan)

| Kategori | File | Action | Prioritas |
|---|---|---|---|
| WA Engine | `services/wa.go` | Merge +50 fungsi baru | 🔴 Kritis |
| History Sync Handler | `handlers/history_sync.go` | Replace + port fungsi JHE | 🔴 Kritis |
| Inbox Infrastructure | `handlers/helpers.go` | Merge (tambah 6 fungsi) | 🔴 Kritis |
| SSE Real-time | `handlers/inbox_events.go` | Merge (redesign hub) | 🔴 Kritis |
| Database | `database/database.go` | Merge (tambah repair) | 🔴 Kritis |
| Models | `models/models.go` | Tambah field + model baru | 🔴 Kritis |
| Auth + CS Guard | `handlers/auth.go` | Merge (tambah CS guard) | 🟠 Tinggi |
| Features/Inbox | `handlers/features.go` | Merge (cursor, profile pic) | 🟠 Tinggi |
| Cleanup | `handlers/cleanup.go` | Merge (lebih lengkap) | 🟠 Tinggi |
| AI Service | `services/ai.go` | Merge (7 fungsi baru) | 🟠 Tinggi |
| Inbox Brief | `services/inbox_brief.go` | Merge (pipeline lebih canggih) | 🟡 Sedang |
| Follow-up | `handlers/followup.go` | Merge (normalize + stop-all) | 🟡 Sedang |
| Schedule | `handlers/schedule.go` | Tambah processDueStatuses | 🟡 Sedang |
| Template | `handlers/template.go` | Merge (hati-hati) | 🟡 Sedang |
| Team/CS | `handlers/team.go` [BARU] | Port dari 1.6-1.7 | 🟡 Sedang |
| Link Preview | `handlers/link_preview.go` [BARU] | Port dari 1.6-1.7 | 🟡 Sedang |
| Inbox Identity | `handlers/inbox_identity.go` [BARU] | Port dari 1.6-1.7 | 🟡 Sedang |
| Main.go | `backend/main.go` | Tambah routes + handlers baru | 🔴 Kritis |
| Frontend | `TeamPanel.tsx` [BARU] | Port dari 1.6-1.7 | 🟡 Sedang |
| Frontend | `Dashboard.tsx` | Tambah Team tab + inbox upgrades | 🟡 Sedang |

---

## 📐 PHASE 1 — Foundation: Models & Database Schema

> **Goal:** Semua model baru dan field baru ada di DB, migrasi aman.
> **Risk:** ⚠️ Data loss jika salah. Harus backup dulu.

### 1.1 Update `models/models.go`

**ChatHistory** — tambah 3 field baru + index baru:
```go
// Tambah setelah FromHuman:
LiveIncoming bool `gorm:"not null;default:false;index:idx_chat_agent_live_cursor,priority:2" json:"-"`

// Tambah di ujung sebelum CreatedAt:
MediaAvailable    bool `gorm:"-" json:"media_available"`
MediaDownloadable bool `gorm:"-" json:"media_downloadable"`
```

**Index baru di ChatHistory:**
```go
ID      uint      `gorm:"primaryKey;index:idx_chat_agent_live_cursor,priority:3" json:"id"`
AgentID uint      `gorm:"index;index:idx_chat_agent_live_cursor,priority:1;index:idx_chat_agent_wa_msg,priority:1;index:idx_chat_agent_sender_time,priority:1" json:"agent_id"`
Sender  string    `gorm:"index;size:32;index:idx_chat_agent_sender_time,priority:2" json:"sender"`
CreatedAt time.Time `gorm:"index:idx_chat_agent_sender_time,priority:3" json:"created_at"`
```

**Agent** — tambah 1 field baru:
```go
// Tambah setelah Number:
InboxOwnerNumber string `gorm:"size:32;index" json:"inbox_owner_number,omitempty"`
```

**Tambah model InboxReadState** (sudah ada di JHE tapi mungkin beda versi):
```go
type InboxReadState struct {
    ID                  uint       `gorm:"primaryKey"`
    AgentID             uint       `gorm:"uniqueIndex:idx_inbox_read_agent_sender,priority:1;not null"`
    Sender              string     `gorm:"uniqueIndex:idx_inbox_read_agent_sender,priority:2;size:32;not null"`
    WhatsAppSynced      bool       `gorm:"not null;default:false"`
    WhatsAppUnreadCount int        `gorm:"not null;default:0"`
    WhatsAppStateAt     *time.Time
    LastReadAt          *time.Time
    LastMsgAt           *time.Time
    UpdatedAt           time.Time
}
```

**Tambah model UserAgentAssignment** (untuk Team CS):
```go
type UserAgentAssignment struct {
    ID       uint `gorm:"primaryKey"`
    TenantID uint `gorm:"uniqueIndex:idx_user_agent_assign,priority:1;not null"`
    UserID   uint `gorm:"uniqueIndex:idx_user_agent_assign,priority:2;not null"`
    AgentID  uint `gorm:"uniqueIndex:idx_user_agent_assign,priority:3;not null"`
}
```

**Tambah model CSActivityLog** (untuk audit trail CS):
```go
type CSActivityLog struct {
    ID        uint      `gorm:"primaryKey"`
    TenantID  uint      `gorm:"index;not null"`
    UserID    uint      `gorm:"index;not null"`
    AgentID   uint      `gorm:"index;not null"`
    Action    string    `gorm:"size:32;not null"` // login, reply, handoff, etc.
    Sender    string    `gorm:"size:32"`
    Meta      string    `gorm:"type:text"`
    CreatedAt time.Time `gorm:"index"`
}
```

**Update User model** — tambah field CS:
```go
// Tambah di User struct:
Active   bool   `gorm:"not null;default:true" json:"active"`
IsCSOnly bool   `gorm:"not null;default:false" json:"-"` // CS user, bukan admin
```

### 1.2 Update `database/database.go`

Tambahkan ke `AutoMigrate`:
- `models.InboxReadState{}`
- `models.UserAgentAssignment{}`
- `models.CSActivityLog{}`

Tambahkan repair functions dari 1.6-1.7:
- `backfillInboxLastMsgAt()` — isi last_msg_at untuk data lama
- `backfillHistoricalDeliveryStatus()` — perbaiki delivery status lama
- `normalizeSenderFields()` — normalisasi format nomor HP
- `MergeLegacyGroupThread()` — merge thread grup duplikat
- `preflightCanonicalChatSchema()` — validasi schema
- `ensureCanonicalChatMessageIDs()` — pastikan message ID unik

**Test:** `go build ./...` harus berhasil.

---

## 📡 PHASE 2 — Core WA Engine

> **Goal:** Engine WhatsApp sekuat 1.6-1.7 tapi tetap punya semua kustom JHE.
> **Risk:** 🔴 Tertinggi. Ini jantung sistem.

### 2.1 Merge `services/wa.go`

Strategi: **Tambahkan fungsi-fungsi baru dari 1.6-1.7 ke wa.go JHE**.

Fungsi yang DITAMBAHKAN (ada di 1.6-1.7, belum ada di JHE):

**Group A — History Sync Engine:**
- `processHistorySync` — proses batch history sync
- `RequestHistorySync` / `RequestDeepHistorySync` — minta history
- `RequestChatCatchUp` / `RequestRecentChatCatchUp` — catch-up
- `RequestTimelineRepair` — repair timeline
- `RequestUnavailableMessage` — minta ulang pesan hilang
- `sendHistoryOnDemand` / `sendFullHistoryOnDemand`
- `addHistoryWaiter` / `removeHistoryWaiter` / `notifyAllHistoryWaiters`
- `finishHistorySync` / `completeHistoryRequest` / `failHistoryRequest`
- `ReserveDeepHistorySync` / `ReserveRecentHistorySync`
- `RunReservedDeepHistorySync` / `RunReservedRecentHistorySync`

**Group B — Read State Sync:**
- `SyncConversationUnread` — sync unread per-konversasi
- `MarkConversationRead` — mark conversation read
- `syncReadStates` — sinkronisasi read states
- `reconcileReadStates` — rekonsiliasi read states
- `scheduleReconciliation`

**Group C — Message ID + Dedup:**
- `SendMessageAndGetID` — kirim + dapat ID
- `SendImmediateTextAndGetID` — kirim langsung + ID
- `SendImmediateReplyAndGetID` — balas langsung + ID
- `SendReplyAndGetID` — balas + ID
- `SendImageAndGetID` / `SendVideoAndGetID`
- `SendDocumentAndGetID`
- `SendSystemMessageAndGetID`
- `sendMessageWithDelayAndGetID`
- `storeResentMessage`

**Group D — Media & Context:**
- `DownloadHistoricalMedia` — download media riwayat
- `EnsureMissingQuotedMessages` — pastikan quoted message ada
- `refreshMissingReplyContext` — refresh context reply yang hilang
- `mergeLegacyGroupThreads` — merge thread grup lama
- `downloadIncomingMedia`

**Group E — Helper Functions Baru:**
- `HistorySyncStatus` — status sync
- `CachedGroups` — cache grup
- `GroupName` — nama grup
- `orderedLiveMessageTime`
- `notifyHistoryChatState`
- `normalizeWASendError` / `waitWASendDelay`
- `SetHistoryChatStateHandler` / `SetHistorySyncHandler` (pindah ke sini)
- `SetWhatsAppReadStateHandler`
- `NormalizeInboxSender`
- `truncateRunes` (rename agar tidak clash)
- Various helper functions: `buildCatchUpHistoryRequest`, `cleanHistoryExportFormat`, dll.

Fungsi JHE yang DIPERTAHANKAN:
- `ApplyLabel` — kustom JHE untuk label WA
- `markSystemSent` / `isSystemSent` / `markSent` — anti-echo JHE
- `parseJIDForWA` — parsing JID kustom

**Update `SetHandlers`** — tambah handler baru:
```go
func SetHistoryChatStateHandler(h HistoryChatStateHandler) { onHistoryChatState = h }
func SetWhatsAppReadStateHandler(h WhatsAppReadStateHandler) { onWAReadState = h }
```

### 2.2 Update `handlers/history_sync.go`

Ini hampir full-replace karena versi 1.6-1.7 jauh lebih canggih. Fungsi JHE yang dipertahankan:
- `OnWAMessageRevoke` — masih dipakai JHE
- `GetHistorySyncStatus` — tetap ada

Fungsi baru dari 1.6-1.7 yang ditambahkan:
- `reconcileUnreadAfterConnect`
- `OnWAHistoryChatState`
- `reconcileStaleInboxAfterConnect`
- `OnWAWhatsAppReadState`
- `repairMergedCustomerLine`
- `repairLegacyOutgoingMessage`
- `splitMergedRemainder`
- `RequestHistorySync` (endpoint handler)

### 2.3 Update `handlers/helpers.go`

Tambahkan semua fungsi baru dari 1.6-1.7 ke file yang sudah ada:
- `ensureInboxReadState`
- `touchInboxLastMsg` (monotonik, atomic)
- `setInboxLastMsgFromWA`
- `advanceInboxWAState` (atomic, anti race-condition)
- `recordIncomingWAUnread`
- `inboxWAEventTime`

### 2.4 Update `handlers/inbox_events.go`

Redesign SSE hub agar lebih canggih:
- Tambah revision-based subscription (`subscribeFrom` dengan `afterRevision` + `replay`)
- Tambah `InboxEvent` struct dengan MessageID
- Tambah `publishIncomingInboxEvent` dengan messageID
- Tambah `publishInboxTypingEvent`
- Tambah `OnWAChatPresence` — typing indicator
- Tambah `OnWAMessageRevoked` — handle revoke dari WA
- Tambah `InboxIncomingCursor` — cursor endpoint
- Tambah `dispatch` + `publishTransient`

Pertahankan kompatibilitas: `PublishInboxEvent` tetap ada sebagai wrapper.

**Test:** `go build ./...` + semua existing tests.

---

## 🔐 PHASE 3 — Auth & CS Management

> **Goal:** Support multi-user CS dengan proper access control.

### 3.1 Update `handlers/auth.go`

Tambahkan fungsi baru:
- `currentUserID(c *gin.Context) uint` — ambil user ID dari token
- `isTenantAdmin(c *gin.Context) bool` — cek apakah admin
- `RequireTenantAdmin()` — middleware admin-only
- `CSRouteGuard()` — guard khusus CS (batasi akses hanya ke agent yang di-assign)

Update fungsi yang berubah:
- `tenantFromToken` — sekarang return `(tenantID, userID, bool)` untuk support CS
- `issueToken` — encode user role + CS assignment
- `issueMediaToken` — support scoped agent ID

Update `main.go`:
- Ganti `auth := api.Group("", handlers.AuthMiddleware())` 
- Jadi: `auth := api.Group("", handlers.AuthMiddleware(), handlers.CSRouteGuard())`

### 3.2 Port `handlers/team.go` (BARU dari 1.6-1.7)

Fitur manajemen tim CS:
- `ListTeamUsers` — daftar CS yang terdaftar
- `CreateTeamUser` — buat akun CS baru
- `UpdateTeamUser` — update (nama, password, agent assignment)
- `DeleteTeamUser` — hapus akun CS
- `ListCSActivity` — log aktivitas CS
- Helper: `validateAssignedAgents`, `replaceUserAssignments`

Register routes di `main.go`:
```go
auth.GET("/team/users", handlers.RequireTenantAdmin(), handlers.ListTeamUsers)
auth.POST("/team/users", handlers.RequireTenantAdmin(), handlers.CreateTeamUser)
auth.PUT("/team/users/:uid", handlers.RequireTenantAdmin(), handlers.UpdateTeamUser)
auth.DELETE("/team/users/:uid", handlers.RequireTenantAdmin(), handlers.DeleteTeamUser)
auth.GET("/team/activity", handlers.RequireTenantAdmin(), handlers.ListCSActivity)
```

**Test:** Build + test login CS, test admin guard.

---

## 📥 PHASE 4 — Inbox & Features Upgrade

> **Goal:** Inbox terasa jauh lebih responsif dan akurat untuk user.

### 4.1 Update `handlers/features.go`

Tambahkan fungsi baru:
- `MarkInboxConversationRead` — versi baru yang proper (dengan InboxReadState)
- `inboxConversationLimit` — limit percakapan per-request
- `parseInboxConversationCursor` — cursor-based pagination
- `enrichConversationReplyPreviews` — enrich reply preview
- `truncateChatPreview` — potong preview
- `ServeProfilePicture` — serve foto profil WA

Update `InboxConversation` — gunakan cursor-based pagination.
Update `InboxContacts` — integrasikan dengan InboxReadState yang baru.

Register route baru di `main.go`:
```go
api.GET("/agents/:id/profile-picture", handlers.ServeProfilePicture)
```

### 4.2 Port `handlers/inbox_identity.go` (BARU)

Identitas per-konversasi untuk inbox multi-CS.

### 4.3 Update `handlers/cleanup.go`

Tambahkan fungsi baru:
- `CleanupOrphanAssignments` — hapus relasi CS-agent yatim
- Update `CleanupBroadcastJunk` — juga bersihkan Contacts dan InboxReadState

Update `main.go`:
```go
handlers.CleanupOrphanAssignments() // panggil di startup
```

### 4.4 Port `handlers/link_preview.go` + `services/link_preview.go` (BARU)

Preview link di chat inbox — user bisa lihat preview URL yang dikirim.

Register route di `main.go`:
```go
auth.GET("/agents/:id/inbox/:sender/link-preview", handlers.GetLinkPreview)
```

---

## 🤖 PHASE 5 — AI & Service Upgrades

> **Goal:** AI lebih akurat, tidak kirim duplikat, lebih kontekstual.

### 5.1 Update `services/ai.go`

Tambahkan fungsi baru dari 1.6-1.7:
- `looksLikeVisualRequest` — deteksi request visual
- `knowledgeAlreadySentInHistory` — hindari kirim KB yang sudah dikirim
- `productAlreadySentInHistory` — hindari kirim produk duplikat
- `isNarrowAttributeQuery` — deteksi query spesifik atribut
- `stripMediaDirectives` — bersihkan directive dari reply
- `resolveChatAttachment` — resolve attachment untuk chat
- `createChatResult` — buat hasil chat yang terstruktur

Pertahankan `extractONGKIRBlock` yang ada di JHE (parsing ongkir kustom).

### 5.2 Update `services/inbox_brief.go`

Merge pipeline AI brief yang jauh lebih canggih dari 1.6-1.7:
- Pipeline: Heuristic → AI Enhancement → Merge
- `BuildConversationBriefHeuristic` — mode heuristik cepat
- `briefTurnsFromMessages` — analisa per-turn
- `classifyCurrentBriefIntent` — klasifikasi intent
- `refineBriefFromChronology` — refine dari kronologi
- `normalizeBriefCollections` — normalisasi koleksi
- Dan +20 helper functions baru

Pertahankan: fungsi JHE yang existing tetap ada.

### 5.3 Update `handlers/followup.go`

Tambahkan:
- `normalizeFollowUpSteps` — validasi + normalisasi steps sebelum save
- `stopActiveFollowUps` — stop semua follow-up aktif untuk kontak
- `fallbackAIFollowUpMessage` — pesan fallback jika AI gagal

Update `saveSteps` — gunakan transaction yang proper.

### 5.4 Update `handlers/schedule.go`

Tambahkan:
- `processDueStatuses` — proses WA status yang terjadwal

### 5.5 Update `handlers/chat.go`

Update knowledge image handling:
- `attachKnowledgeImageURLs` — inject URL gambar ke KB response
- `saveKnowledgeImage` — handler save gambar KB yang lebih robust

Register route baru:
```go
api.GET("/agents/:id/knowledge/:kid/image", handlers.ServeKnowledgeImage)
```

---

## 🧹 PHASE 6 — Cleanup & Route Registration

> **Goal:** Semua route baru terdaftar, startup bersih.

### 6.1 Update `backend/main.go`

Tambahkan:
```go
// Handler registrations (baru):
services.SetHistoryChatStateHandler(handlers.OnWAHistoryChatState)
services.SetWhatsAppReadStateHandler(handlers.OnWAWhatsAppReadState)
services.SetMessageRevokeHandler(handlers.OnWAMessageRevoked) // nama baru

// Startup cleanup (baru):
handlers.CleanupOrphanAssignments()

// Routes baru:
auth.POST("/agents/:id/history-sync", handlers.RequestHistorySync)
auth.POST("/agents/:id/inbox/reset", handlers.ResetAgentInbox)
auth.POST("/agents/:id/conversation/read", handlers.MarkInboxConversationRead)
auth.POST("/agents/:id/conversation/brief", handlers.RefreshConversationBrief)
auth.GET("/agents/:id/inbox/incoming", handlers.InboxIncomingCursor)
auth.GET("/team/users", handlers.RequireTenantAdmin(), handlers.ListTeamUsers)
auth.POST("/team/users", handlers.RequireTenantAdmin(), handlers.CreateTeamUser)
auth.PUT("/team/users/:uid", handlers.RequireTenantAdmin(), handlers.UpdateTeamUser)
auth.DELETE("/team/users/:uid", handlers.RequireTenantAdmin(), handlers.DeleteTeamUser)
auth.GET("/team/activity", handlers.RequireTenantAdmin(), handlers.ListCSActivity)
api.GET("/agents/:id/profile-picture", handlers.ServeProfilePicture)
api.GET("/agents/:id/knowledge/:kid/image", handlers.ServeKnowledgeImage)

// CSRouteGuard:
auth := api.Group("", handlers.AuthMiddleware(), handlers.CSRouteGuard())
```

---

## 🎨 PHASE 7 — Frontend Update

> **Goal:** UI mencerminkan semua fitur baru, UX lebih mulus.

### 7.1 Port `components/TeamPanel.tsx` (BARU dari 1.6-1.7)

Panel manajemen CS user — sudah lengkap dari 1.6-1.7, tinggal port.

### 7.2 Update `pages/Dashboard.tsx`

Tambahkan:
- Import `TeamPanel`
- Tambah tab `team` di nav: "Pengguna CS"
- Render `{tab === 'team' && !isCS && <TeamPanel />}`
- Update `ContactsPanel` props untuk support `onScheduleBroadcast`
- Update InboxPanel dengan event handling baru

### 7.3 Update `hooks.ts`

Tambahkan hooks baru:
- `useTeamUsers` / `useCreateTeamUser` / `useUpdateTeamUser` / `useDeleteTeamUser`
- `useCSActivity`
- `useInboxReadState`
- `useLinkPreview`
- `useProfilePicture`

### 7.4 Update `types.ts`

Tambahkan tipe baru:
- `TeamUser` 
- `CSActivityLog`
- `InboxReadState`
- `LinkPreview`

---

## ✅ PHASE 8 — Testing & Validation

> **Goal:** Tidak ada bug, semua fitur lama dan baru jalan.

### 8.1 Build Test
```bash
go build ./...
```

### 8.2 Unit Tests
```bash
go test ./backend/... -v
```

Tests yang harus pass:
- `learning_date_test.go` — date parsing
- `learning_patterns_test.go` — pattern extraction
- `learning_status_test.go` — status check
- `broadcast_rotation_test.go` — rotasi broadcast
- `flow_test.go` — flow logic
- `history_sync_test.go` — history sync (updated)
- `inbox_events_test.go` — SSE events (updated)
- `contacts_test.go` — contacts
- Semua test lainnya

### 8.3 Frontend Build
```bash
cd frontend && npm run build
```

### 8.4 Smoke Test Checklist
- [ ] Login admin berhasil
- [ ] Connect WA agent → QR muncul → Linked
- [ ] Inbox: pesan masuk muncul realtime
- [ ] Inbox: typing indicator muncul
- [ ] Inbox: mark read berfungsi
- [ ] Inbox: unread badge akurat
- [ ] Inbox: history sync setelah connect
- [ ] Broadcast: kirim ke beberapa nomor
- [ ] Follow-up: buat + enroll + trigger
- [ ] Learning: jalankan + lihat pola
- [ ] Shipping (Mengantar): cek ongkir
- [ ] Pipeline: atur stage + label
- [ ] Team CS: buat akun CS, assign agent
- [ ] Media Assets: upload + gunakan di chat

---

## 🎯 User POV: Apa yang Terasa Beda

### Sebelum Update (JHE sekarang):
- Inbox agak lambat refresh
- Badge unread kadang salah
- History saat baru connect kosong / lama muncul
- Typing indicator tidak ada
- Pesan terhapus (revoke) masih keliatan
- CS harus pakai akun admin semua

### Setelah Update (JHE + 1.6-1.7):
- ✅ **Inbox real-time**: Pesan baru muncul < 1 detik tanpa refresh
- ✅ **Typing indicator**: "Sedang mengetik..." muncul saat customer ketik
- ✅ **Badge unread akurat**: Angka unread benar, sync dengan WA
- ✅ **History langsung ada**: Buka inbox langsung lihat riwayat chat lama
- ✅ **Revoke langsung hilang**: Pesan terhapus di WA langsung hilang di inbox
- ✅ **Foto profil**: Muncul di inbox (sebelumnya tidak ada)
- ✅ **AI lebih pintar**: Tidak kirim info yang sudah pernah dikirim
- ✅ **Brief konversasi lebih akurat**: Ringkasan chat di kontak lebih relevan
- ✅ **CS Multi-user**: Bisa buat akun khusus CS, batasi akses per-nomor WA
- ✅ **Link preview**: URL yang dikirim di chat tampil preview-nya

---

## ⚠️ Risiko & Mitigasi

| Risiko | Mitigasi |
|---|---|
| `wa.go` merge conflict | Tambahkan fungsi 1.6-1.7 di blok terpisah, jangan ubah fungsi JHE existing |
| DB migration merusak data | Test di DB test dulu, backup sebelum jalankan |
| `tenantFromToken` signature berubah | Update semua tempat yang memanggilnya |
| `InboxReadState` belum ada di JHE | Buat dengan AutoMigrate, data lama aman |
| CSRouteGuard memblokir akses yang seharusnya bebas | Test semua route setelah implementasi |

---

## 📅 Estimasi Per Phase

| Phase | Kompleksitas | Estimasi |
|---|---|---|
| Phase 1 (Models + DB) | Tinggi | Sesi 1 |
| Phase 2 (WA Engine) | Sangat Tinggi | Sesi 2-3 |
| Phase 3 (Auth + Team) | Tinggi | Sesi 4 |
| Phase 4 (Inbox Features) | Tinggi | Sesi 5 |
| Phase 5 (AI + Services) | Sedang | Sesi 6 |
| Phase 6 (Main.go) | Sedang | Sesi 7 |
| Phase 7 (Frontend) | Tinggi | Sesi 8 |
| Phase 8 (Testing) | Sedang | Sesi 9 |

