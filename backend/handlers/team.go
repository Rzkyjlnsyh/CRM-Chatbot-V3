package handlers

import (
	"log"
	"regexp"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// csUsernamePattern = aturan username akun CS (huruf, angka, titik, strip,
// garis bawah; 3–64 karakter). Port dari v4 agar konsisten di semua jalur.
var csUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{3,64}$`)

// teamUserResponse membentuk respons user CS yang aman (tanpa password).
func teamUserResponse(u models.User, agentIDs []uint) gin.H {
	return gin.H{
		"id":         u.ID,
		"username":   u.Username,
		"name":       u.Name,
		"email":      u.Email,
		"phone":      u.Phone,
		"active":     u.Active,
		"is_cs_only": u.IsCSOnly,
		"agent_ids":  agentIDs,
	}
}

// getUserAgentIDs mengembalikan daftar agent ID yang di-assign ke user.
func getUserAgentIDs(tenantID, userID uint) []uint {
	var assigns []models.UserAgentAssignment
	database.DB.Where("tenant_id = ? AND user_id = ?", tenantID, userID).Find(&assigns)
	ids := make([]uint, 0, len(assigns))
	for _, a := range assigns {
		ids = append(ids, a.AgentID)
	}
	return ids
}

// validateAssignedAgents memastikan semua agentID yang dikirim adalah milik tenant ini.
func validateAssignedAgents(tenantID uint, agentIDs []uint) ([]uint, bool) {
	if len(agentIDs) == 0 {
		return nil, true
	}
	var count int64
	database.DB.Model(&models.Agent{}).
		Where("id IN ? AND tenant_id = ?", agentIDs, tenantID).
		Count(&count)
	if int(count) != len(agentIDs) {
		return nil, false
	}
	return agentIDs, true
}

// replaceUserAssignments mengganti semua assignment agent untuk user tertentu.
func replaceUserAssignments(tx *gorm.DB, tenantID, userID uint, agentIDs []uint) error {
	if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Delete(&models.UserAgentAssignment{}).Error; err != nil {
		return err
	}
	for _, aid := range agentIDs {
		if err := tx.Create(&models.UserAgentAssignment{
			TenantID: tenantID,
			UserID:   userID,
			AgentID:  aid,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ListTeamUsers mengembalikan daftar semua user CS dalam tenant.
// GET /api/team/users
func ListTeamUsers(c *gin.Context) {
	tid := currentTenantID(c)
	var users []models.User
	database.DB.Where("tenant_id = ? AND is_cs_only = true", tid).
		Order("id asc").Find(&users)
	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		agentIDs := getUserAgentIDs(tid, u.ID)
		out = append(out, teamUserResponse(u, agentIDs))
	}
	c.JSON(200, gin.H{"data": out})
}

// CreateTeamUser membuat akun CS baru dalam tenant.
// POST /api/team/users
func CreateTeamUser(c *gin.Context) {
	tid := currentTenantID(c)
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		AgentIDs []uint `json:"agent_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Name = strings.TrimSpace(req.Name)
	if req.Username == "" || req.Password == "" {
		c.JSON(400, gin.H{"error": "Username dan password wajib diisi"})
		return
	}
	if !csUsernamePattern.MatchString(req.Username) {
		c.JSON(400, gin.H{"error": "Username minimal 3 karakter dan hanya boleh berisi huruf, angka, titik, garis bawah, atau strip"})
		return
	}
	if len(req.Password) < 8 {
		c.JSON(400, gin.H{"error": "Password minimal 8 karakter"})
		return
	}
	// Cek username sudah dipakai.
	var existing models.User
	if database.DB.Where("username = ?", req.Username).First(&existing).Error == nil {
		c.JSON(409, gin.H{"error": "Username sudah dipakai"})
		return
	}
	// Validasi agent IDs.
	agentIDs, ok := validateAssignedAgents(tid, req.AgentIDs)
	if !ok {
		c.JSON(400, gin.H{"error": "Satu atau lebih agent tidak valid"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(500, gin.H{"error": "Gagal memproses password"})
		return
	}

	var newUser models.User
	if txErr := database.DB.Transaction(func(tx *gorm.DB) error {
		newUser = models.User{
			Username:      req.Username,
			Password:      string(hash),
			Name:          req.Name,
			Email:         req.Email,
			Phone:         req.Phone,
			TenantID:      &tid,
			Role:          "cs",
			IsCSOnly:      true,
			Active:        true,
			EmailVerified: true, // CS user tidak perlu verifikasi email
		}
		if err := tx.Create(&newUser).Error; err != nil {
			return err
		}
		return replaceUserAssignments(tx, tid, newUser.ID, agentIDs)
	}); txErr != nil {
		log.Printf("[team] gagal buat CS user: %v", txErr)
		c.JSON(500, gin.H{"error": "Gagal membuat akun CS"})
		return
	}

	c.JSON(201, gin.H{"data": teamUserResponse(newUser, agentIDs)})
}

// UpdateTeamUser memperbarui data user CS (nama, password, assignment agent, status aktif).
// PUT /api/team/users/:uid
func UpdateTeamUser(c *gin.Context) {
	tid := currentTenantID(c)
	targetUID := parseUintParam(c.Param("uid"))
	if targetUID == 0 {
		c.JSON(400, gin.H{"error": "ID user tidak valid"})
		return
	}
	var user models.User
	if database.DB.Where("id = ? AND tenant_id = ? AND is_cs_only = true", targetUID, tid).
		First(&user).Error != nil {
		c.JSON(404, gin.H{"error": "User CS tidak ditemukan"})
		return
	}
	var req struct {
		Name     *string `json:"name"`
		Password *string `json:"password"`
		Phone    *string `json:"phone"`
		Active   *bool   `json:"active"`
		AgentIDs *[]uint `json:"agent_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	// Validasi agent IDs jika diberikan.
	var agentIDs []uint
	if req.AgentIDs != nil {
		var ok bool
		agentIDs, ok = validateAssignedAgents(tid, *req.AgentIDs)
		if !ok {
			c.JSON(400, gin.H{"error": "Satu atau lebih agent tidak valid"})
			return
		}
	} else {
		agentIDs = getUserAgentIDs(tid, user.ID)
	}

	if txErr := database.DB.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{}
		if req.Name != nil {
			updates["name"] = strings.TrimSpace(*req.Name)
		}
		if req.Phone != nil {
			updates["phone"] = strings.TrimSpace(*req.Phone)
		}
		if req.Active != nil {
			updates["active"] = *req.Active
		}
		if req.Password != nil {
			if len(*req.Password) < 8 {
				return gorm.ErrInvalidData
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			updates["password"] = string(hash)
		}
		if len(updates) > 0 {
			if err := tx.Model(&user).Updates(updates).Error; err != nil {
				return err
			}
		}
		if req.AgentIDs != nil {
			return replaceUserAssignments(tx, tid, user.ID, agentIDs)
		}
		return nil
	}); txErr != nil {
		if txErr == gorm.ErrInvalidData {
			c.JSON(400, gin.H{"error": "Password minimal 8 karakter"})
			return
		}
		log.Printf("[team] gagal update CS user %d: %v", targetUID, txErr)
		c.JSON(500, gin.H{"error": "Gagal memperbarui akun CS"})
		return
	}
	// Reload user.
	database.DB.First(&user, targetUID)
	c.JSON(200, gin.H{"data": teamUserResponse(user, agentIDs)})
}

// DeleteTeamUser menghapus akun CS dan semua assignment-nya.
// DELETE /api/team/users/:uid
func DeleteTeamUser(c *gin.Context) {
	tid := currentTenantID(c)
	targetUID := parseUintParam(c.Param("uid"))
	if targetUID == 0 {
		c.JSON(400, gin.H{"error": "ID user tidak valid"})
		return
	}
	var user models.User
	if database.DB.Where("id = ? AND tenant_id = ? AND is_cs_only = true", targetUID, tid).
		First(&user).Error != nil {
		c.JSON(404, gin.H{"error": "User CS tidak ditemukan"})
		return
	}
	if txErr := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND user_id = ?", tid, targetUID).
			Delete(&models.UserAgentAssignment{}).Error; err != nil {
			return err
		}
		return tx.Delete(&user).Error
	}); txErr != nil {
		log.Printf("[team] gagal hapus CS user %d: %v", targetUID, txErr)
		c.JSON(500, gin.H{"error": "Gagal menghapus akun CS"})
		return
	}
	c.JSON(200, gin.H{"message": "Akun CS berhasil dihapus"})
}

// ListCSActivity mengembalikan log aktivitas CS (diurutkan terbaru dulu).
// GET /api/team/activity
func ListCSActivity(c *gin.Context) {
	tid := currentTenantID(c)
	limit := 100
	var logs []models.CSActivityLog
	database.DB.Where("tenant_id = ?", tid).
		Order("created_at desc").Limit(limit).Find(&logs)

	// Enrich dengan nama user.
	type logEntry struct {
		models.CSActivityLog
		UserName string `json:"user_name"`
	}
	out := make([]logEntry, 0, len(logs))
	userCache := map[uint]string{}
	for _, l := range logs {
		name, ok := userCache[l.UserID]
		if !ok {
			var u models.User
			if database.DB.Select("name, username").First(&u, l.UserID).Error == nil {
				if u.Name != "" {
					name = u.Name
				} else {
					name = u.Username
				}
			}
			userCache[l.UserID] = name
		}
		out = append(out, logEntry{CSActivityLog: l, UserName: name})
	}
	c.JSON(200, gin.H{"data": out})
}

// LogCSActivity mencatat satu aktivitas CS (dipanggil dari handler lain).
func LogCSActivity(tenantID, userID, agentID uint, action, sender, meta string) {
	_ = database.DB.Create(&models.CSActivityLog{
		TenantID:  tenantID,
		UserID:    userID,
		AgentID:   agentID,
		Action:    action,
		Sender:    sender,
		Meta:      meta,
		CreatedAt: time.Now(),
	}).Error
}

// logCSActivity = wrapper LogCSActivity yang membaca user & tenant dari context
// request (dipanggil di jalur Inbox: buka chat, kirim balasan, kirim media).
func logCSActivity(c *gin.Context, agentID uint, sender, action, detail string) {
	uid := currentUserID(c)
	tid := currentTenantID(c)
	if uid == 0 || tid == 0 || agentID == 0 {
		return
	}
	LogCSActivity(tid, uid, agentID, action, sender, detail)
}

// CleanupOrphanAssignments menghapus UserAgentAssignment yang user atau agent-nya sudah tidak ada.
// Dipanggil saat startup.
func CleanupOrphanAssignments() {
	// Hapus assignment yang user-nya tidak ada lagi.
	res := database.DB.Exec(`
		DELETE FROM user_agent_assignments
		WHERE user_id NOT IN (SELECT id FROM users)
	`)
	if res.Error != nil {
		log.Printf("[cleanup] orphan assignment (user) error: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("[cleanup] %d user_agent_assignments yatim (user tidak ada) dihapus", res.RowsAffected)
	}

	// Hapus assignment yang agent-nya tidak ada lagi.
	res = database.DB.Exec(`
		DELETE FROM user_agent_assignments
		WHERE agent_id NOT IN (SELECT id FROM agents)
	`)
	if res.Error != nil {
		log.Printf("[cleanup] orphan assignment (agent) error: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("[cleanup] %d user_agent_assignments yatim (agent tidak ada) dihapus", res.RowsAffected)
	}
}
