package httpserver

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"server/internal/app/user"
	"server/internal/domain/shared"
	domainUser "server/internal/domain/user"
)

const (
	accessTokenTTL  = 8 * time.Hour
	refreshTokenTTL = 30 * 24 * time.Hour // 30 days
)

// tokenEntry holds an access token with expiry and its paired refresh token.
type tokenEntry struct {
	UserID       string
	Email        string
	ExpiresAt    time.Time
	RefreshToken string
}

// AuthHandler handles /user/auth/* endpoints.
type AuthHandler struct {
	userSvc *user.Service

	mu            sync.RWMutex
	tokens        map[string]*tokenEntry // accessToken → entry
	refreshTokens map[string]string      // refreshToken → accessToken (reverse lookup)
}

func NewAuthHandler(userSvc *user.Service) *AuthHandler {
	return &AuthHandler{
		userSvc:       userSvc,
		tokens:        make(map[string]*tokenEntry),
		refreshTokens: make(map[string]string),
	}
}

func (h *AuthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /user/auth/google/signup", h.signup)
	mux.HandleFunc("POST /user/auth/google/login", h.login)
	mux.HandleFunc("POST /user/auth/refresh", h.refresh)
	mux.HandleFunc("GET /user/me", h.me)
}

type authRequest struct {
	GoogleIDToken string `json:"google_id_token"`
	FirebaseToken string `json:"firebase_token"`
	Email         string `json:"email"`
	DisplayName   string `json:"display_name"`
	PromptText    string `json:"prompt_text"`
}

type authResponse struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	Plan         string `json:"plan"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"` // unix seconds
}

func (h *AuthHandler) signup(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Use real email from Firebase if provided, otherwise fallback.
	email := req.Email
	if email == "" {
		email = req.FirebaseToken + "@samams.dev"
	}
	displayName := req.DisplayName
	if displayName == "" {
		displayName = "Local User"
	}

	u, err := h.userSvc.CreateOrLoginUserViaProvider(r.Context(), user.CreateOrLoginUserViaProviderCommand{
		TenantID:    shared.TenantID("default"),
		Email:       email,
		DisplayName: displayName,
		Plan:        domainUser.PlanFree,
		GoogleSub:   req.GoogleIDToken,
		FirebaseUID: req.FirebaseToken,
	})
	if err != nil {
		MapDomainError(w, err)
		return
	}

	// Generate access + refresh token pair.
	accessToken := string(shared.GenerateID())
	refreshToken := string(shared.GenerateID())
	now := time.Now()

	h.mu.Lock()
	h.tokens[accessToken] = &tokenEntry{
		UserID:       string(u.ID),
		Email:        u.Email,
		ExpiresAt:    now.Add(accessTokenTTL),
		RefreshToken: refreshToken,
	}
	h.refreshTokens[refreshToken] = accessToken
	h.mu.Unlock()

	WriteOK(w, authResponse{
		ID:           string(u.ID),
		Email:        u.Email,
		DisplayName:  u.DisplayName,
		Plan:         planString(u.Plan),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    now.Add(accessTokenTTL).Unix(),
	})
}

func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	h.signup(w, r)
}

func (h *AuthHandler) refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := DecodeJSON(r, &req); err != nil || req.RefreshToken == "" {
		WriteError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Find access token by refresh token.
	oldAccessToken, ok := h.refreshTokens[req.RefreshToken]
	if !ok {
		WriteError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	oldEntry, ok := h.tokens[oldAccessToken]
	if !ok {
		delete(h.refreshTokens, req.RefreshToken)
		WriteError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	userID := oldEntry.UserID
	email := oldEntry.Email

	// Delete old tokens.
	delete(h.tokens, oldAccessToken)
	delete(h.refreshTokens, req.RefreshToken)

	// Issue new pair.
	newAccessToken := string(shared.GenerateID())
	newRefreshToken := string(shared.GenerateID())
	now := time.Now()

	h.tokens[newAccessToken] = &tokenEntry{
		UserID:       userID,
		Email:        email,
		ExpiresAt:    now.Add(accessTokenTTL),
		RefreshToken: newRefreshToken,
	}
	h.refreshTokens[newRefreshToken] = newAccessToken

	WriteOK(w, authResponse{
		ID:           userID,
		Email:        email,
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
		ExpiresAt:    now.Add(accessTokenTTL).Unix(),
	})
}

func (h *AuthHandler) me(w http.ResponseWriter, r *http.Request) {
	userID := h.ExtractUser(r)
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}

	WriteOK(w, map[string]string{
		"id":    userID,
		"email": userID,
	})
}

// ExtractUser returns the user ID from the Authorization header.
// Returns "" if token is missing, invalid, or expired.
func (h *AuthHandler) ExtractUser(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	token := strings.TrimPrefix(auth, "Bearer ")

	h.mu.RLock()
	defer h.mu.RUnlock()

	entry, ok := h.tokens[token]
	if !ok {
		return ""
	}
	if time.Now().After(entry.ExpiresAt) {
		return "" // expired → 401
	}
	return entry.UserID
}

func planString(p domainUser.Plan) string {
	switch p {
	case domainUser.PlanFree:
		return "free"
	case domainUser.PlanPro:
		return "pro"
	case domainUser.PlanEnterprise:
		return "enterprise"
	default:
		return "unknown"
	}
}
