package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go-practice/internal/config"
	"go-practice/view"
	"html/template"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/amazon"
	"golang.org/x/oauth2/google"
)

type userRole string

const (
	roleAdmin userRole = "admin"
	roleUser  userRole = "user"
)

type authManager struct {
	database      *sql.DB
	redis         *redis.Client
	template      *template.Template
	secureCookies bool
	oauth         config.OAuthSettings
}

type sessionData struct {
	Role userRole `json:"role"`
}

func newAuthManager(database *sql.DB, redisClient *redis.Client, secureCookies bool, oauthSettings config.OAuthSettings) *authManager {
	return &authManager{
		database:      database,
		redis:         redisClient,
		template:      template.Must(template.ParseFS(view.Files, "login.html", "register.html")),
		secureCookies: secureCookies,
		oauth:         oauthSettings,
	}
}

func (manager *authManager) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/login", manager.login)
	mux.HandleFunc("/logout", manager.logout)
	mux.HandleFunc("/register", manager.register)
	mux.HandleFunc("/oauth/google", manager.oauthStart("google"))
	mux.HandleFunc("/oauth/google/callback", manager.oauthCallback("google"))
	mux.HandleFunc("/oauth/amazon", manager.oauthStart("amazon"))
	mux.HandleFunc("/oauth/amazon/callback", manager.oauthCallback("amazon"))
}

func (manager *authManager) login(response http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet {
		if err := manager.template.Execute(response, map[string]string{"CSRFToken": manager.csrfToken(request)}); err != nil {
			http.Error(response, "login page rendering failed", http.StatusInternalServerError)
		}
		return
	}
	if request.Method != http.MethodPost {
		response.Header().Set("Allow", "GET, POST")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := strings.TrimSpace(request.FormValue("username"))
	password := request.FormValue("password")
	if manager.isRateLimited(request, username) {
		http.Error(response, "ログイン試行が多すぎます。しばらくしてから再試行してください", http.StatusTooManyRequests)
		return
	}
	role, err := manager.authenticate(request.Context(), username, password)
	if err != nil || role == "" {
		manager.recordLoginFailure(request, username)
		http.Error(response, "ユーザー名またはパスワードが正しくありません", http.StatusUnauthorized)
		return
	}
	manager.clearLoginFailures(request, username)
	sessionID, err := manager.createSession(request.Context(), role)
	if err != nil {
		http.Error(response, "ログイン処理に失敗しました", http.StatusInternalServerError)
		return
	}
	http.SetCookie(response, &http.Cookie{
		Name: "go_practice_session", Value: sessionID,
		Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: manager.secureCookies, MaxAge: 8 * 60 * 60,
	})
	redirect := request.FormValue("redirect")
	if redirect == "" || !strings.HasPrefix(redirect, "/") || strings.HasPrefix(redirect, "//") {
		redirect = "/shop"
	}
	http.Redirect(response, request, redirect, http.StatusSeeOther)
}

func (manager *authManager) register(response http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet {
		if err := manager.template.ExecuteTemplate(response, "register.html", map[string]string{"CSRFToken": manager.csrfToken(request)}); err != nil {
			http.Error(response, "registration page rendering failed", http.StatusInternalServerError)
		}
		return
	}
	if request.Method != http.MethodPost {
		response.Header().Set("Allow", "GET, POST")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := strings.TrimSpace(request.FormValue("username"))
	password := request.FormValue("password")
	if !validRegistration(username, password) {
		http.Error(response, "ユーザー名またはパスワードが不正です", http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil || manager.database == nil {
		http.Error(response, "ユーザー登録に失敗しました", http.StatusInternalServerError)
		return
	}
	if _, err := manager.database.ExecContext(request.Context(), "INSERT INTO USERS (username, password_hash, role) VALUES (?, ?, 'user')", username, string(hash)); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			http.Error(response, "そのユーザー名は既に使用されています", http.StatusConflict)
			return
		}
		http.Error(response, "ユーザー登録に失敗しました", http.StatusInternalServerError)
		return
	}
	http.Redirect(response, request, "/login", http.StatusSeeOther)
}

func validRegistration(username, password string) bool {
	return len(username) >= 3 && len(username) <= 100 && len(password) >= 8 && len(password) <= 72 && !strings.ContainsAny(username, " \t\r\n")
}

func (manager *authManager) oauthStart(provider string) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		configuration, ok := manager.oauthConfig(provider)
		if !ok || manager.redis == nil {
			http.Error(response, "OAuthプロバイダーが設定されていません", http.StatusNotImplemented)
			return
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			http.Error(response, "OAuth開始に失敗しました", http.StatusInternalServerError)
			return
		}
		state := hex.EncodeToString(raw)
		if err := manager.redis.Set(request.Context(), "oauth-state:"+state, provider, 10*time.Minute).Err(); err != nil {
			http.Error(response, "OAuth開始に失敗しました", http.StatusInternalServerError)
			return
		}
		http.Redirect(response, request, configuration.AuthCodeURL(state, oauth2.AccessTypeOnline), http.StatusFound)
	}
}

func (manager *authManager) oauthCallback(provider string) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		configuration, ok := manager.oauthConfig(provider)
		state := request.URL.Query().Get("state")
		if !ok || manager.redis == nil {
			http.Error(response, "OAuthプロバイダーが設定されていません", http.StatusNotImplemented)
			return
		}
		if manager.database == nil {
			http.Error(response, "OAuth認証に必要なデータベースが設定されていません", http.StatusInternalServerError)
			return
		}
		expected, err := manager.redis.GetDel(request.Context(), "oauth-state:"+state).Result()
		if err != nil || expected != provider {
			http.Error(response, "OAuth stateが不正です", http.StatusBadRequest)
			return
		}
		token, err := configuration.Exchange(request.Context(), request.URL.Query().Get("code"))
		if err != nil {
			http.Error(response, "OAuth認証に失敗しました", http.StatusUnauthorized)
			return
		}
		userinfoURL := "https://www.googleapis.com/oauth2/v2/userinfo"
		if provider == "amazon" {
			userinfoURL = "https://api.amazon.com/user/profile"
		}
		userinfoRequest, err := http.NewRequestWithContext(request.Context(), http.MethodGet, userinfoURL, nil)
		if err != nil {
			http.Error(response, "OAuth認証に失敗しました", http.StatusInternalServerError)
			return
		}
		userinfoResponse, err := configuration.Client(request.Context(), token).Do(userinfoRequest)
		if err != nil {
			http.Error(response, "OAuth認証に失敗しました", http.StatusUnauthorized)
			return
		}
		defer userinfoResponse.Body.Close()
		if userinfoResponse.StatusCode != http.StatusOK {
			http.Error(response, "OAuth認証に失敗しました", http.StatusUnauthorized)
			return
		}
		var profile struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		}
		if err := json.NewDecoder(userinfoResponse.Body).Decode(&profile); err != nil || profile.ID == "" {
			http.Error(response, "OAuthユーザー情報を取得できません", http.StatusUnauthorized)
			return
		}
		username := provider + "_" + profile.ID
		if _, err := manager.database.ExecContext(request.Context(), "INSERT INTO USERS (username, password_hash, role, oauth_provider, oauth_subject, email) VALUES (?, '', 'user', ?, ?, ?) ON DUPLICATE KEY UPDATE email = VALUES(email)", username, provider, profile.ID, profile.Email); err != nil {
			http.Error(response, "OAuthユーザー登録に失敗しました", http.StatusInternalServerError)
			return
		}
		role, err := manager.userRole(request.Context(), username)
		if err != nil {
			http.Error(response, "OAuthログインに失敗しました", http.StatusInternalServerError)
			return
		}
		sessionID, err := manager.createSession(request.Context(), role)
		if err != nil {
			http.Error(response, "ログイン処理に失敗しました", http.StatusInternalServerError)
			return
		}
		http.SetCookie(response, &http.Cookie{Name: "go_practice_session", Value: sessionID, Path: "/", HttpOnly: true, Secure: manager.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: 8 * 60 * 60})
		http.Redirect(response, request, "/shop", http.StatusSeeOther)
	}
}

func (manager *authManager) oauthConfig(provider string) (*oauth2.Config, bool) {
	switch provider {
	case "google":
		if manager.oauth.GoogleClientID == "" || manager.oauth.GoogleClientSecret == "" || manager.oauth.GoogleRedirectURL == "" {
			return nil, false
		}
		return &oauth2.Config{ClientID: manager.oauth.GoogleClientID, ClientSecret: manager.oauth.GoogleClientSecret, Endpoint: google.Endpoint, RedirectURL: manager.oauth.GoogleRedirectURL, Scopes: []string{"openid", "email", "profile"}}, true
	case "amazon":
		if manager.oauth.AmazonClientID == "" || manager.oauth.AmazonClientSecret == "" || manager.oauth.AmazonRedirectURL == "" {
			return nil, false
		}
		return &oauth2.Config{ClientID: manager.oauth.AmazonClientID, ClientSecret: manager.oauth.AmazonClientSecret, Endpoint: amazon.Endpoint, RedirectURL: manager.oauth.AmazonRedirectURL, Scopes: []string{"profile"}}, true
	default:
		return nil, false
	}
}

func (manager *authManager) userRole(ctx context.Context, username string) (userRole, error) {
	var role string
	err := manager.database.QueryRowContext(ctx, "SELECT role FROM USERS WHERE username = ? AND is_active = 1", username).Scan(&role)
	return userRole(role), err
}

func (manager *authManager) logout(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		response.Header().Set("Allow", http.MethodPost)
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if cookie, err := request.Cookie("go_practice_session"); err == nil && manager.redis != nil {
		_ = manager.redis.Del(request.Context(), "session:"+cookie.Value).Err()
	}
	http.SetCookie(response, &http.Cookie{Name: "go_practice_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: manager.secureCookies, SameSite: http.SameSiteLaxMode})
	http.Redirect(response, request, "/login", http.StatusSeeOther)
}

func (manager *authManager) authenticate(ctx context.Context, username, password string) (userRole, error) {
	if manager.database == nil {
		return "", sql.ErrConnDone
	}
	var role string
	var passwordHash string
	err := manager.database.QueryRowContext(ctx, "SELECT role, password_hash FROM USERS WHERE username = ? AND is_active = 1", username).Scan(&role, &passwordHash)
	if err != nil {
		return "", err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return "", err
	}
	return userRole(role), nil
}

func (manager *authManager) createSession(ctx context.Context, role userRole) (string, error) {
	if manager.redis == nil {
		return "", redis.Nil
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	sessionID := hex.EncodeToString(raw)
	data, err := json.Marshal(sessionData{Role: role})
	if err != nil {
		return "", err
	}
	return sessionID, manager.redis.Set(ctx, "session:"+sessionID, data, 8*time.Hour).Err()
}

func (manager *authManager) role(request *http.Request) userRole {
	cookie, err := request.Cookie("go_practice_session")
	if err != nil || manager.redis == nil {
		return ""
	}
	value, err := manager.redis.Get(request.Context(), "session:"+cookie.Value).Result()
	if err != nil {
		return ""
	}
	var session sessionData
	if err := json.Unmarshal([]byte(value), &session); err != nil {
		return ""
	}
	if session.Role != roleAdmin && session.Role != roleUser {
		return ""
	}
	return session.Role
}

func (manager *authManager) csrfToken(request *http.Request) string {
	cookie, err := request.Cookie("go_practice_csrf")
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (manager *authManager) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		manager.ensureCSRFToken(response, request)
		if request.URL.Path == "/health" || request.URL.Path == "/ready" ||
			strings.HasPrefix(request.URL.Path, "/static/") ||
			strings.HasPrefix(request.URL.Path, "/uploads/") {
			next.ServeHTTP(response, request)
			return
		}
		publicAuth := request.URL.Path == "/login" || request.URL.Path == "/register" ||
			request.URL.Path == "/oauth/google" || request.URL.Path == "/oauth/google/callback" ||
			request.URL.Path == "/oauth/amazon" || request.URL.Path == "/oauth/amazon/callback"
		if publicAuth {
			if request.URL.Path != "/login" && isStateChanging(request.Method) && !manager.validCSRF(request) {
				http.Error(response, "CSRF validation failed", http.StatusForbidden)
				return
			}
			next.ServeHTTP(response, request)
			return
		}
		if isStateChanging(request.Method) && !manager.validCSRF(request) {
			http.Error(response, "CSRF validation failed", http.StatusForbidden)
			return
		}
		role := manager.role(request)
		if role == "" {
			redirect := "/login?redirect=" + url.QueryEscape(request.URL.RequestURI())
			http.Redirect(response, request, redirect, http.StatusSeeOther)
			return
		}
		if role == roleUser && request.URL.Path != "/shop" && !strings.HasPrefix(request.URL.Path, "/shop/") {
			http.Error(response, "管理者権限が必要です", http.StatusForbidden)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func isStateChanging(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func (manager *authManager) ensureCSRFToken(response http.ResponseWriter, request *http.Request) {
	if _, err := request.Cookie("go_practice_csrf"); err == nil {
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return
	}
	http.SetCookie(response, &http.Cookie{
		Name: "go_practice_csrf", Value: hex.EncodeToString(raw), Path: "/",
		HttpOnly: false, Secure: manager.secureCookies, SameSite: http.SameSiteLaxMode,
		MaxAge: 8 * 60 * 60,
	})
}

func (manager *authManager) validCSRF(request *http.Request) bool {
	cookie, cookieErr := request.Cookie("go_practice_csrf")
	header := request.Header.Get("X-CSRF-Token")
	if cookieErr == nil && header != "" && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) == 1 {
		return true
	}
	formToken := request.FormValue("csrf_token")
	if cookieErr == nil && formToken != "" && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(formToken)) == 1 {
		return true
	}
	origin := request.Header.Get("Origin")
	if origin != "" {
		parsed, err := url.Parse(origin)
		return err == nil && parsed.Host == request.Host
	}
	referer := request.Header.Get("Referer")
	if referer != "" {
		parsed, err := url.Parse(referer)
		return err == nil && parsed.Host == request.Host
	}
	return false
}

func (manager *authManager) rateKey(request *http.Request, username string) string {
	ip := request.Header.Get("X-Forwarded-For")
	if comma := strings.IndexByte(ip, ','); comma >= 0 {
		ip = ip[:comma]
	}
	if ip == "" {
		ip = request.RemoteAddr
	}
	if host, _, err := net.SplitHostPort(strings.TrimSpace(ip)); err == nil {
		ip = host
	}
	return "login-failure:" + strings.TrimSpace(ip) + ":" + strings.ToLower(username)
}

func (manager *authManager) isRateLimited(request *http.Request, username string) bool {
	if manager.redis == nil {
		return false
	}
	count, err := manager.redis.Get(request.Context(), manager.rateKey(request, username)).Int()
	return err == nil && count >= 5
}

func (manager *authManager) recordLoginFailure(request *http.Request, username string) {
	if manager.redis == nil {
		return
	}
	key := manager.rateKey(request, username)
	count, err := manager.redis.Incr(request.Context(), key).Result()
	if err == nil && count == 1 {
		_ = manager.redis.Expire(request.Context(), key, 15*time.Minute).Err()
	}
}

func (manager *authManager) clearLoginFailures(request *http.Request, username string) {
	if manager.redis != nil {
		_ = manager.redis.Del(request.Context(), manager.rateKey(request, username)).Err()
	}
}
