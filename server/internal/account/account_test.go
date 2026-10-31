package account

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"killa/server/internal/store"
)

type testAPI struct {
	mux *http.ServeMux
	svc *Service
}

func newTestAPI(t *testing.T) *testAPI {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "account.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := New(st)
	mux := http.NewServeMux()
	svc.Register(mux)
	return &testAPI{mux: mux, svc: svc}
}

func (a *testAPI) do(t *testing.T, method, path string, body any, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	a.mux.ServeHTTP(rec, req)
	return rec.Result()
}

func decodeProfile(t *testing.T, resp *http.Response) profileJSON {
	t.Helper()
	var out struct {
		Profile profileJSON `json:"profile"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Profile
}

func cookieFrom(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == CookieName {
			return c
		}
	}
	return nil
}

func TestGuestAndSession(t *testing.T) {
	api := newTestAPI(t)

	resp := api.do(t, http.MethodPost, "/api/auth", map[string]string{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("auth status = %d", resp.StatusCode)
	}
	profile := decodeProfile(t, resp)
	cookie := cookieFrom(resp)
	if cookie == nil || !profile.IsGuest || profile.Username != "" {
		t.Fatalf("guest profile/cookie = %+v %+v", profile, cookie)
	}

	resp = api.do(t, http.MethodGet, "/api/session", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("session status = %d", resp.StatusCode)
	}
	if got := decodeProfile(t, resp); got.ID != profile.ID {
		t.Fatalf("session profile = %s, want %s", got.ID, profile.ID)
	}
}

func TestRegisterLoginAndLogout(t *testing.T) {
	api := newTestAPI(t)
	creds := map[string]string{"username": "Alice", "password": "password123"}

	resp := api.do(t, http.MethodPost, "/api/auth", creds)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register status = %d", resp.StatusCode)
	}
	profile := decodeProfile(t, resp)
	cookie := cookieFrom(resp)
	if profile.IsGuest || profile.Username != "Alice" {
		t.Fatalf("registered profile = %+v", profile)
	}

	resp = api.do(t, http.MethodPost, "/api/auth/logout", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d", resp.StatusCode)
	}

	resp = api.do(t, http.MethodPost, "/api/auth", map[string]string{"username": "Alice", "password": "wrongpass"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d", resp.StatusCode)
	}

	resp = api.do(t, http.MethodPost, "/api/auth", creds)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	var out struct {
		Created bool `json:"created"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Created {
		t.Fatal("existing account reported as created")
	}
}

func TestUpgradeGuestAndProfile(t *testing.T) {
	api := newTestAPI(t)
	resp := api.do(t, http.MethodPost, "/api/auth", map[string]string{})
	cookie := cookieFrom(resp)

	resp = api.do(t, http.MethodPost, "/api/account/upgrade", map[string]string{
		"username": "GuestUp", "password": "password123",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upgrade status = %d", resp.StatusCode)
	}
	if got := decodeProfile(t, resp); got.IsGuest || got.Username != "GuestUp" {
		t.Fatalf("upgraded = %+v", got)
	}

	resp = api.do(t, http.MethodPost, "/api/profile", map[string]any{
		"settings": map[string]any{"sensitivity": 0.5},
		"loadout":  "smg",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("profile update status = %d", resp.StatusCode)
	}
	got := decodeProfile(t, resp)
	if got.Loadout != "smg" || !bytes.Contains(got.Settings, []byte("sensitivity")) {
		t.Fatalf("profile = %+v settings=%s", got, got.Settings)
	}

	// The same credentials now log in from a fresh browser.
	resp = api.do(t, http.MethodPost, "/api/auth", map[string]string{
		"username": "GuestUp", "password": "password123",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login after upgrade = %d", resp.StatusCode)
	}
}

func TestDeleteAccount(t *testing.T) {
	api := newTestAPI(t)
	creds := map[string]string{"username": "DeleteMe", "password": "password123"}
	resp := api.do(t, http.MethodPost, "/api/auth", creds)
	cookie := cookieFrom(resp)

	resp = api.do(t, http.MethodDelete, "/api/account", map[string]string{"password": "wrongpass"}, cookie)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong delete password = %d", resp.StatusCode)
	}

	resp = api.do(t, http.MethodDelete, "/api/account", map[string]string{"password": "password123"}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}

	resp = api.do(t, http.MethodGet, "/api/session", nil, cookie)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("session after delete = %d", resp.StatusCode)
	}
}

func TestChangePassword(t *testing.T) {
	api := newTestAPI(t)
	creds := map[string]string{"username": "PassUser", "password": "password123"}
	resp := api.do(t, http.MethodPost, "/api/auth", creds)
	cookie := cookieFrom(resp)

	resp = api.do(t, http.MethodPost, "/api/account/password", map[string]string{
		"currentPassword": "password123",
		"newPassword":     "newpassword123",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("password change = %d", resp.StatusCode)
	}

	if resp = api.do(t, http.MethodPost, "/api/auth", creds); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password login = %d", resp.StatusCode)
	}
	resp = api.do(t, http.MethodPost, "/api/auth", map[string]string{
		"username": "PassUser", "password": "newpassword123",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new password login = %d", resp.StatusCode)
	}
}

func TestCrossOriginRejected(t *testing.T) {
	api := newTestAPI(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth", bytes.NewBufferString(`{}`))
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	api.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
