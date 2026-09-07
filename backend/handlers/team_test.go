package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/gin-gonic/gin"
	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTeamGuardTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:team-guard-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.UserAgentAssignment{}, &models.Agent{}, &models.Tenant{}); err != nil {
		t.Fatal(err)
	}
	tid := uint(1)
	db.Create(&models.User{ID: 1, TenantID: &tid, Username: "owner", IsSuperAdmin: true, IsCSOnly: false})
	db.Create(&models.User{ID: 2, TenantID: &tid, Username: "cs.andi", IsSuperAdmin: false, IsCSOnly: true})
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
}

func TestCSRouteGuard(t *testing.T) {
	setupTeamGuardTest(t)
	tests := []struct {
		name       string
		userID     uint
		method     string
		path       string
		route      string
		wantStatus int
	}{
		{
			name: "cs may send from assigned inbox", userID: 2, method: http.MethodPost,
			path: "/api/agents/12/send", route: "/api/agents/:id/send", wantStatus: http.StatusNoContent,
		},
		{
			name: "cs may stream assigned inbox events", userID: 2, method: http.MethodGet,
			path: "/api/agents/12/inbox/events", route: "/api/agents/:id/inbox/events", wantStatus: http.StatusNoContent,
		},
		{
			name: "cs may poll assigned inbox incoming cursor", userID: 2, method: http.MethodGet,
			path: "/api/agents/12/inbox/incoming-cursor", route: "/api/agents/:id/inbox/incoming-cursor", wantStatus: http.StatusNoContent,
		},
		{
			name: "cs may submit assigned inbox diagnostics", userID: 2, method: http.MethodPost,
			path: "/api/agents/12/inbox/client-debug", route: "/api/agents/:id/inbox/client-debug", wantStatus: http.StatusNoContent,
		},
		{
			name: "cs cannot create whatsapp agent", userID: 2, method: http.MethodPost,
			path: "/api/agents", route: "/api/agents", wantStatus: http.StatusForbidden,
		},
		{
			name: "cs cannot touch knowledge admin route", userID: 2, method: http.MethodGet,
			path: "/api/agents/12/knowledge", route: "/api/agents/:id/knowledge", wantStatus: http.StatusForbidden,
		},
		{
			name: "owner may use admin route", userID: 1, method: http.MethodPost,
			path: "/api/agents", route: "/api/agents", wantStatus: http.StatusNoContent,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Handle(tc.method, tc.route,
				func(c *gin.Context) {
					c.Set("user_id", tc.userID)
					c.Set("tenant_id", uint(1))
					c.Next()
				},
				CSRouteGuard(),
				func(c *gin.Context) { c.Status(http.StatusNoContent) },
			)
			request := httptest.NewRequest(tc.method, tc.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tc.wantStatus, response.Body.String())
			}
		})
	}
}

func TestCSUsernamePattern(t *testing.T) {
	valid := []string{"cs.andi", "operator_01", "cs-malam"}
	invalid := []string{"ab", "cs malam", "cs@kantor"}
	for _, username := range valid {
		if !csUsernamePattern.MatchString(username) {
			t.Errorf("username %q seharusnya valid", username)
		}
	}
	for _, username := range invalid {
		if csUsernamePattern.MatchString(username) {
			t.Errorf("username %q seharusnya ditolak", username)
		}
	}
}
