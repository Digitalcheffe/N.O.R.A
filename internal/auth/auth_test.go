package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/auth"
	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-key"

func TestGenerateAndValidateToken(t *testing.T) {
	token, err := auth.GenerateToken("user-1", "admin", testSecret)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := auth.ValidateToken(token, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Errorf("UserID: got %q, want %q", claims.UserID, "user-1")
	}
	if claims.Role != "admin" {
		t.Errorf("Role: got %q, want %q", claims.Role, "admin")
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	token, err := auth.GenerateToken("user-1", "admin", testSecret)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	_, err = auth.ValidateToken(token, "wrong-secret")
	if err == nil {
		t.Fatal("expected error with wrong secret, got nil")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	// Build an already-expired token manually.
	claims := auth.Claims{
		UserID: "user-1",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	_, err = auth.ValidateToken(signed, testSecret)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestValidateToken_Malformed(t *testing.T) {
	_, err := auth.ValidateToken("not.a.token", testSecret)
	if err == nil {
		t.Fatal("expected error for malformed token, got nil")
	}
}

// --- middleware tests ---

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestRequireAuth_ValidBearerToken(t *testing.T) {
	token, _ := auth.GenerateToken("user-1", "member", testSecret)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	auth.RequireAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestRequireAuth_ValidCookie(t *testing.T) {
	token, _ := auth.GenerateToken("user-1", "member", testSecret)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "nora_session", Value: token})
	rr := httptest.NewRecorder()

	auth.RequireAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestRequireAuth_MissingToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	auth.RequireAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bad.token.here")
	rr := httptest.NewRecorder()

	auth.RequireAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestRequireAdmin_AdminToken(t *testing.T) {
	token, _ := auth.GenerateToken("user-1", "admin", testSecret)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	auth.RequireAdmin(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestRequireAdmin_MemberToken(t *testing.T) {
	token, _ := auth.GenerateToken("user-1", "member", testSecret)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	auth.RequireAdmin(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestContextHelpers(t *testing.T) {
	token, _ := auth.GenerateToken("user-42", "admin", testSecret)

	var capturedUserID, capturedRole string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = auth.UserID(r.Context())
		capturedRole = auth.Role(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	auth.RequireAuth(testSecret)(handler).ServeHTTP(rr, req)

	if capturedUserID != "user-42" {
		t.Errorf("UserID: got %q, want %q", capturedUserID, "user-42")
	}
	if capturedRole != "admin" {
		t.Errorf("Role: got %q, want %q", capturedRole, "admin")
	}
}
