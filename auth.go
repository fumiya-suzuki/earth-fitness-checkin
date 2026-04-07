package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	adminSessionCookieName = "admin_session"
	adminSessionTTL        = 12 * time.Hour
)

type adminAuthConfig struct {
	username string
	password string
}

type adminSession struct {
	expiresAt time.Time
}

type adminAuth struct {
	config       adminAuthConfig
	mu           sync.Mutex
	sessions     map[string]adminSession
	loginTmpl    *template.Template
	cookieSecure bool
}

func loadAdminAuthConfig() (adminAuthConfig, error) {
	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")

	if username == "" || password == "" {
		return adminAuthConfig{}, errors.New("ADMIN_USERNAME and ADMIN_PASSWORD must be set")
	}

	return adminAuthConfig{
		username: username,
		password: password,
	}, nil
}

func newAdminAuth(config adminAuthConfig) *adminAuth {
	return &adminAuth{
		config:       config,
		sessions:     make(map[string]adminSession),
		loginTmpl:    template.Must(template.ParseFiles(filepathJoin("public", "admin_login.html"))),
		cookieSecure: strings.EqualFold(os.Getenv("ADMIN_COOKIE_SECURE"), "true"),
	}
}

func (a *adminAuth) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.isAuthenticated(r) {
			http.Redirect(w, r, a.loginURL(r), http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *adminAuth) handleLogin(w http.ResponseWriter, r *http.Request) {
	normalizedNext := a.normalizeNext(r.URL.Query().Get("next"))

	switch r.Method {
	case http.MethodGet:
		if a.isAuthenticated(r) {
			http.Redirect(w, r, normalizedNext, http.StatusSeeOther)
			return
		}
		a.renderLogin(w, normalizedNext, "")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			a.renderLogin(w, normalizedNext, "入力を読み取れませんでした。")
			return
		}

		username := r.FormValue("username")
		password := r.FormValue("password")
		next := a.normalizeNext(r.FormValue("next"))

		if subtle.ConstantTimeCompare([]byte(username), []byte(a.config.username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(password), []byte(a.config.password)) != 1 {
			a.renderLogin(w, next, "ログイン情報が正しくありません。")
			return
		}

		token, err := newAdminSessionToken()
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		a.mu.Lock()
		a.cleanupExpiredSessionsLocked(time.Now())
		a.sessions[token] = adminSession{expiresAt: time.Now().Add(adminSessionTTL)}
		a.mu.Unlock()

		http.SetCookie(w, &http.Cookie{
			Name:     adminSessionCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   a.shouldUseSecureCookie(r),
			MaxAge:   int(adminSessionTTL.Seconds()),
		})

		http.Redirect(w, r, next, http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *adminAuth) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cookie, err := r.Cookie(adminSessionCookieName); err == nil && cookie.Value != "" {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
	}

	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.shouldUseSecureCookie(r),
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (a *adminAuth) isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(adminSessionCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}

	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()

	a.cleanupExpiredSessionsLocked(now)

	session, ok := a.sessions[cookie.Value]
	if !ok || session.expiresAt.Before(now) {
		delete(a.sessions, cookie.Value)
		return false
	}

	session.expiresAt = now.Add(adminSessionTTL)
	a.sessions[cookie.Value] = session
	return true
}

func (a *adminAuth) cleanupExpiredSessionsLocked(now time.Time) {
	for token, session := range a.sessions {
		if session.expiresAt.Before(now) {
			delete(a.sessions, token)
		}
	}
}

func (a *adminAuth) renderLogin(w http.ResponseWriter, next, errorMessage string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = a.loginTmpl.Execute(w, struct {
		Next         string
		ErrorMessage string
	}{
		Next:         next,
		ErrorMessage: errorMessage,
	})
}

func (a *adminAuth) loginURL(r *http.Request) string {
	next := a.normalizeNext(r.URL.RequestURI())
	return "/admin/login?next=" + url.QueryEscape(next)
}

func (a *adminAuth) normalizeNext(next string) string {
	if next == "" {
		return "/admin/visits"
	}
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/admin/visits"
	}
	if strings.HasPrefix(next, "/admin/login") {
		return "/admin/visits"
	}
	return next
}

func (a *adminAuth) shouldUseSecureCookie(r *http.Request) bool {
	if a.cookieSecure {
		return true
	}
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func newAdminSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func filepathJoin(parts ...string) string {
	return strings.Join(parts, string(os.PathSeparator))
}
