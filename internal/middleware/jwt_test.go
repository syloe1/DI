package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
)

var testSecret = []byte("test-secret")

type fakeRevoker struct {
	revoked bool
}

func (f *fakeRevoker) IsRevoked(_ context.Context, _ string, _ uint, _ time.Time) (bool, error) {
	return f.revoked, nil
}

func (f *fakeRevoker) Revoke(_ context.Context, _ string, _ uint, _ time.Time) error { return nil }

func signToken(t *testing.T, expiresAt time.Time) string {
	t.Helper()
	claims := &Claims{
		UserId:   7,
		Username: "tester",
		Role:     "user",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(testSecret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func newAuthRouter(revoker TokenRevoker) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", JWTAuth(testSecret, revoker), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id": c.GetUint("userID"),
			"token":   c.GetString("token"),
		})
	})
	return r
}

func TestJWTAuthAllowsValidToken(t *testing.T) {
	r := newAuthRouter(&fakeRevoker{})
	token := signToken(t, time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestJWTAuthRejectsRevokedToken(t *testing.T) {
	r := newAuthRouter(&fakeRevoker{revoked: true})
	token := signToken(t, time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestJWTAuthRejectsMissingToken(t *testing.T) {
	r := newAuthRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}
