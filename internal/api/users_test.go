package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

func newUsersRouter(t *testing.T) http.Handler {
	t.Helper()
	db := newTestDB(t)
	userRepo := repo.NewUserRepo(db)
	settingsRepo := repo.NewSettingsRepo(db)
	h := api.NewUsersHandler(userRepo, settingsRepo)
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

func createUser(t *testing.T, router http.Handler, email, password, role string) models.User {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"email": email, "password": password, "role": role})
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createUser: expected 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var u models.User
	if err := json.NewDecoder(rr.Body).Decode(&u); err != nil {
		t.Fatalf("createUser decode: %v", err)
	}
	return u
}

// --- List ---

func TestListUsers_Empty(t *testing.T) {
	router := newUsersRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var resp struct {
		Data  []models.User `json:"data"`
		Total int           `json:"total"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Total != 0 {
		t.Errorf("expected total=0 got %d", resp.Total)
	}
}

func TestListUsers_ReturnsAll(t *testing.T) {
	router := newUsersRouter(t)
	createUser(t, router, "alice@example.com", "password1", "admin")
	createUser(t, router, "bob@example.com", "password2", "member")

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	var resp struct {
		Data  []models.User `json:"data"`
		Total int           `json:"total"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Total != 2 {
		t.Errorf("expected total=2 got %d", resp.Total)
	}
}

// --- Create ---

func TestCreateUser_HappyPath(t *testing.T) {
	router := newUsersRouter(t)
	u := createUser(t, router, "test@example.com", "password123", "member")

	if u.ID == "" {
		t.Error("expected non-empty ID")
	}
	if u.Email != "test@example.com" {
		t.Errorf("expected email got %q", u.Email)
	}
	if u.Role != "member" {
		t.Errorf("expected role=member got %q", u.Role)
	}
}

func TestCreateUser_MissingEmail(t *testing.T) {
	router := newUsersRouter(t)
	body := bytes.NewBufferString(`{"password":"pw"}`)
	req := httptest.NewRequest(http.MethodPost, "/users", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestCreateUser_MissingPassword(t *testing.T) {
	router := newUsersRouter(t)
	body := bytes.NewBufferString(`{"email":"a@b.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/users", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestCreateUser_InvalidRole(t *testing.T) {
	router := newUsersRouter(t)
	body := bytes.NewBufferString(`{"email":"a@b.com","password":"pw","role":"superuser"}`)
	req := httptest.NewRequest(http.MethodPost, "/users", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

// --- Delete ---

func TestDeleteUser_HappyPath(t *testing.T) {
	router := newUsersRouter(t)
	u := createUser(t, router, "del@example.com", "password123", "member")

	req := httptest.NewRequest(http.MethodDelete, "/users/"+u.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 got %d", rr.Code)
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	router := newUsersRouter(t)
	req := httptest.NewRequest(http.MethodDelete, "/users/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}
