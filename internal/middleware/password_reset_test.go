package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestPasswordResetTokenOnlyAllowsPasswordWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{UserID: "admin", Role: "admin", Purpose: "password_reset"}).SignedString([]byte("reset-test"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/api/me", 204}, {"GET", "/api/me?detail=1", 204},
		{"POST", "/api/me/password", 204}, {"POST", "/api/me/logout", 204},
		{"PATCH", "/api/me", 403}, {"POST", "/api/me", 403},
		{"GET", "/api/me/password", 403}, {"GET", "/api/me/temporary-password", 403},
		{"POST", "/api/me/temporary-password", 403}, {"GET", "/api/admin/settings", 403},
		{"GET", "/api/stream/movie", 403}, {"GET", "/api/me/../admin/settings", 403},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := gin.New()
			r.Use(AuthRequired("reset-test"))
			r.NoRoute(func(c *gin.Context) { c.Status(204) })
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
