package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseAPIPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/?page=3&per_page=999", nil)

	page := parseAPIPage(ctx, 50, 200)
	if page.Page != 3 || page.PerPage != 200 || page.Offset != 400 {
		t.Fatalf("pagination tidak sesuai: %+v", page)
	}
}

func TestNewSignedWebhookRequest(t *testing.T) {
	body := `{"event":"webhook.test"}`
	secret := "whsec_test"
	req, err := newSignedWebhookRequest("https://example.com/webhook", secret, strings.NewReader(body))
	if err != nil {
		t.Fatalf("membuat request: %v", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got := req.Header.Get("X-Signature"); got != want {
		t.Fatalf("signature = %q, ingin %q", got, want)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
}
