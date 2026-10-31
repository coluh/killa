// Package account implements anonymous profiles, username/password sign-in,
// cookie sessions and the profile HTTP API.
package account

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"

	"killa/server/internal/store"
)

const (
	CookieName = "killa_session"
	SessionTTL = 30 * 24 * time.Hour

	minPassword = 8
	maxPassword = 128
	maxBody     = 8 << 10
)

var usernameRE = regexp.MustCompile(`^[A-Za-z0-9_]{2,16}$`)

// Service owns the auth and profile API.
type Service struct {
	store   *store.Store
	now     func() time.Time
	limiter *limiter
	guests  *limiter
}

func New(st *store.Store) *Service {
	return &Service{
		store:   st,
		now:     time.Now,
		limiter: newLimiter(10, time.Minute),
		guests:  newLimiter(60, time.Minute),
	}
}

// Resolve maps a raw session cookie value to its profile.
func (s *Service) Resolve(token string) (*store.Profile, error) {
	if token == "" {
		return nil, store.ErrNotFound
	}
	return s.store.SessionProfile(hashToken(token), s.now())
}

// Register wires the API routes.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("POST /api/auth", s.handleAuth)
	mux.HandleFunc("GET /api/me", s.handleMe)
	mux.HandleFunc("POST /api/profile", s.handleProfile)
	mux.HandleFunc("POST /api/account/upgrade", s.handleUpgrade)
	mux.HandleFunc("POST /api/account/password", s.handlePassword)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("DELETE /api/account", s.handleDelete)
}

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	Profile profileJSON `json:"profile"`
	Created bool        `json:"created"`
}

type profileJSON struct {
	ID          string          `json:"id"`
	Username    string          `json:"username"`
	DisplayName string          `json:"displayName"`
	IsGuest     bool            `json:"isGuest"`
	CreatedAt   int64           `json:"createdAt"`
	Loadout     string          `json:"loadout"`
	Settings    json.RawMessage `json:"settings"`
	Stats       store.Stats     `json:"stats"`
}

type matchJSON struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"roomId"`
	EndedAt   time.Time `json:"endedAt"`
	Score     [2]int    `json:"score"`
	Winner    int       `json:"winner"`
	Team      int       `json:"team"`
	Kills     int       `json:"kills"`
	Deaths    int       `json:"deaths"`
	Assists   int       `json:"assists"`
	Damage    int       `json:"damage"`
	Headshots int       `json:"headshots"`
}

func toProfileJSON(p *store.Profile) profileJSON {
	settings := json.RawMessage(p.Settings)
	if len(settings) == 0 || !json.Valid(settings) {
		settings = json.RawMessage(`{}`)
	}
	return profileJSON{
		ID:          p.ID,
		Username:    p.Username,
		DisplayName: p.DisplayName,
		IsGuest:     p.IsGuest,
		CreatedAt:   p.CreatedAt.Unix(),
		Loadout:     p.Loadout,
		Settings:    settings,
		Stats:       p.Stats,
	}
}

func (s *Service) handleSession(w http.ResponseWriter, r *http.Request) {
	profile, _, err := s.current(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "未登录")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": toProfileJSON(profile)})
}

func (s *Service) handleAuth(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(w, r) {
		return
	}

	var req authRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	ip := clientIP(r)

	username := strings.TrimSpace(req.Username)
	if username == "" {
		if !s.guests.allow("ip:" + ip) {
			writeError(w, http.StatusTooManyRequests, "创建游客档案太频繁，请稍后再试")
			return
		}
		profile, err := s.createGuest()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "创建档案失败")
			return
		}
		if err := s.issueSession(w, r, profile.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "创建会话失败")
			return
		}
		writeJSON(w, http.StatusOK, authResponse{Profile: toProfileJSON(profile), Created: true})
		return
	}

	if !usernameRE.MatchString(username) {
		writeError(w, http.StatusBadRequest, "用户名需为 2–16 位字母、数字或下划线")
		return
	}
	if !s.limiter.allow("ip:" + ip) {
		writeError(w, http.StatusTooManyRequests, "尝试太频繁，请稍后再试")
		return
	}
	if len(req.Password) < minPassword || len(req.Password) > maxPassword {
		writeError(w, http.StatusBadRequest, "密码长度需为 8–128 位")
		return
	}
	if !s.limiter.allow("user:" + strings.ToLower(username)) {
		writeError(w, http.StatusTooManyRequests, "尝试太频繁，请稍后再试")
		return
	}

	lower := strings.ToLower(username)
	profile, hash, err := s.store.FindByUsername(lower)
	if err == nil {
		if !verifyPassword(req.Password, hash) {
			writeError(w, http.StatusUnauthorized, "用户名或密码错误")
			return
		}
		if err := s.issueSession(w, r, profile.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "创建会话失败")
			return
		}
		writeJSON(w, http.StatusOK, authResponse{Profile: toProfileJSON(profile), Created: false})
		return
	}
	if !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "数据库错误")
		return
	}

	hash, err = hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密码处理失败")
		return
	}
	profile, err = s.store.CreateUser(randomID(), username, lower, username, hash, s.now())
	if err != nil {
		// Someone may have registered the same name between lookup and insert.
		profile, hash, err = s.store.FindByUsername(lower)
		if err != nil || !verifyPassword(req.Password, hash) {
			writeError(w, http.StatusConflict, "用户名已被占用")
			return
		}
		if err := s.issueSession(w, r, profile.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "创建会话失败")
			return
		}
		writeJSON(w, http.StatusOK, authResponse{Profile: toProfileJSON(profile), Created: false})
		return
	}
	if err := s.issueSession(w, r, profile.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "创建会话失败")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{Profile: toProfileJSON(profile), Created: true})
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	profile, _, err := s.current(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "未登录")
		return
	}
	recent, err := s.store.RecentMatches(profile.ID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取战绩失败")
		return
	}
	matches := make([]matchJSON, 0, len(recent))
	for _, m := range recent {
		matches = append(matches, matchJSON{
			ID: m.ID, RoomID: m.RoomID, EndedAt: m.EndedAt,
			Score: m.Score, Winner: m.Winner, Team: m.Team,
			Kills: m.Kills, Deaths: m.Deaths, Assists: m.Assists,
			Damage: m.Damage, Headshots: m.Headshots,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"profile": toProfileJSON(profile),
		"recent":  matches,
	})
}

func (s *Service) handleProfile(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(w, r) {
		return
	}
	profile, _, err := s.current(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "未登录")
		return
	}

	var req struct {
		Settings json.RawMessage `json:"settings"`
		Loadout  string          `json:"loadout"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	settings := profile.Settings
	if len(req.Settings) > 0 {
		if !json.Valid(req.Settings) {
			writeError(w, http.StatusBadRequest, "设置格式错误")
			return
		}
		settings = string(req.Settings)
	}
	loadout := profile.Loadout
	if req.Loadout != "" {
		if req.Loadout != "ar" && req.Loadout != "smg" {
			writeError(w, http.StatusBadRequest, "未知配装")
			return
		}
		loadout = req.Loadout
	}

	if err := s.store.UpdateSettings(profile.ID, settings, loadout); err != nil {
		writeError(w, http.StatusInternalServerError, "保存失败")
		return
	}
	updated, err := s.store.ProfileByID(profile.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取档案失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": toProfileJSON(updated)})
}

func (s *Service) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(w, r) {
		return
	}
	profile, _, err := s.current(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "未登录")
		return
	}
	if !profile.IsGuest {
		writeError(w, http.StatusBadRequest, "已经是正式账号")
		return
	}

	var req authRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	username := strings.TrimSpace(req.Username)
	if !usernameRE.MatchString(username) {
		writeError(w, http.StatusBadRequest, "用户名需为 2–16 位字母、数字或下划线")
		return
	}
	ip := clientIP(r)
	if !s.limiter.allow("ip:" + ip) {
		writeError(w, http.StatusTooManyRequests, "尝试太频繁，请稍后再试")
		return
	}
	if len(req.Password) < minPassword || len(req.Password) > maxPassword {
		writeError(w, http.StatusBadRequest, "密码长度需为 8–128 位")
		return
	}
	if !s.limiter.allow("user:" + strings.ToLower(username)) {
		writeError(w, http.StatusTooManyRequests, "尝试太频繁，请稍后再试")
		return
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密码处理失败")
		return
	}
	if err := s.store.SetCredentials(profile.ID, username, strings.ToLower(username), username, hash); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeError(w, http.StatusConflict, "用户名已被占用")
			return
		}
		writeError(w, http.StatusInternalServerError, "绑定失败")
		return
	}
	updated, err := s.store.ProfileByID(profile.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取档案失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": toProfileJSON(updated)})
}

func (s *Service) handlePassword(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(w, r) {
		return
	}
	profile, tokenHash, err := s.current(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "未登录")
		return
	}
	if profile.IsGuest {
		writeError(w, http.StatusBadRequest, "游客账号还没有密码")
		return
	}

	var req struct {
		Current string `json:"currentPassword"`
		New     string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	_, hash, err := s.store.FindByUsername(strings.ToLower(profile.Username))
	if err != nil || !verifyPassword(req.Current, hash) {
		writeError(w, http.StatusUnauthorized, "当前密码错误")
		return
	}
	if len(req.New) < minPassword || len(req.New) > maxPassword {
		writeError(w, http.StatusBadRequest, "新密码长度需为 8–128 位")
		return
	}
	newHash, err := hashPassword(req.New)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密码处理失败")
		return
	}
	if err := s.store.SetPassword(profile.ID, newHash); err != nil {
		writeError(w, http.StatusInternalServerError, "修改失败")
		return
	}
	_ = s.store.DeleteOtherSessions(profile.ID, tokenHash)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(w, r) {
		return
	}
	if _, tokenHash, err := s.current(r); err == nil {
		_ = s.store.DeleteSession(tokenHash)
	}
	clearCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(w, r) {
		return
	}
	profile, _, err := s.current(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "未登录")
		return
	}

	if !profile.IsGuest {
		var req struct {
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		_, hash, err := s.store.FindByUsername(strings.ToLower(profile.Username))
		if err != nil || !verifyPassword(req.Password, hash) {
			writeError(w, http.StatusUnauthorized, "密码错误")
			return
		}
	}

	if err := s.store.DeleteProfile(profile.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "删除失败")
		return
	}
	clearCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Session helpers.

func (s *Service) current(r *http.Request) (*store.Profile, string, error) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return nil, "", store.ErrNotFound
	}
	tokenHash := hashToken(c.Value)
	profile, err := s.store.SessionProfile(tokenHash, s.now())
	if err != nil {
		return nil, "", err
	}
	return profile, tokenHash, nil
}

func (s *Service) issueSession(w http.ResponseWriter, r *http.Request, profileID string) error {
	token := randomToken(32)
	now := s.now()
	if err := s.store.CreateSession(profileID, hashToken(token), now, now.Add(SessionTTL)); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionTTL.Seconds()),
	})
	return nil
}

func (s *Service) createGuest() (*store.Profile, error) {
	return s.store.CreateGuest(randomID(), randomGuestName(), s.now())
}

func clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// Request helpers.

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func sameOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || !strings.EqualFold(u.Host, r.Host) {
		writeError(w, http.StatusForbidden, "跨站请求被拒绝")
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, ok := strings.Cut(xff, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// IDs and tokens.

func randomID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func randomGuestName() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return "Guest-" + code[:4]
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Passwords.

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	const (
		timeCost = 3
		memory   = 32 * 1024
		threads  = 2
		keyLen   = 32
	)
	key := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, keyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory, timeCost, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var memory uint32
	var timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// Rate limiting.

type limiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*bucket
}

type bucket struct {
	count int
	reset time.Time
}

func newLimiter(limit int, window time.Duration) *limiter {
	return &limiter{limit: limit, window: window, buckets: make(map[string]*bucket)}
}

func (l *limiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.buckets) > 4096 {
		for k, b := range l.buckets {
			if now.After(b.reset) {
				delete(l.buckets, k)
			}
		}
	}

	b := l.buckets[key]
	if b == nil || now.After(b.reset) {
		l.buckets[key] = &bucket{count: 1, reset: now.Add(l.window)}
		return true
	}
	b.count++
	return b.count <= l.limit
}
