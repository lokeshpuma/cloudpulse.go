package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cloudpulse/cloudpulse-go/backend/internal/auth"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/middleware"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/models"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/repository"
	"github.com/cloudpulse/cloudpulse-go/backend/pkg/response"
)

const refreshCookieName = "cloudpulse_refresh"

func (a *API) setSessionCookies(w http.ResponseWriter, refreshToken, csrfToken string, remember bool) {
	maxAge := int(a.tokens.RefreshTTL().Seconds())
	if !remember {
		maxAge = int((24 * time.Hour).Seconds())
	}
	sameSite := http.SameSiteLaxMode
	if a.cfg.CookieSecure {
		sameSite = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    refreshToken,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: sameSite,
		MaxAge:   maxAge,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.CSRFCookieName,
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false, // readable by JS for X-CSRF-Token double-submit
		Secure:   a.cfg.CookieSecure,
		SameSite: sameSite,
		MaxAge:   maxAge,
	})
}

func (a *API) clearSessionCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		MaxAge:   -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:   middleware.CSRFCookieName,
		Value:  "",
		Path:   "/",
		Secure: a.cfg.CookieSecure,
		MaxAge: -1,
	})
}

func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string `json:"name"`
		Email            string `json:"email"`
		Password         string `json:"password"`
		OrganizationName string `json:"organization_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.OrganizationName = strings.TrimSpace(req.OrganizationName)

	if req.Name == "" || len(req.Name) > 120 {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Full name is required")
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Valid email address is required")
		return
	}
	if req.OrganizationName == "" || len(req.OrganizationName) > 120 {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Organization name is required")
		return
	}
	if err := auth.ValidatePasswordStrength(req.Password); err != nil {
		response.Error(w, r, http.StatusBadRequest, "WEAK_PASSWORD", err.Error())
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to process registration")
		return
	}

	org, user, err := a.store.CreateOrgAndAdmin(r.Context(), req.OrganizationName, req.Name, req.Email, hash)
	if errors.Is(err, repository.ErrConflict) {
		response.Error(w, r, http.StatusConflict, "EMAIL_EXISTS", "An account with this email already exists")
		return
	}
	if err != nil {
		a.logger.Error("register failed", "error", err)
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not create account")
		return
	}

	accessToken, exp, err := a.tokens.GenerateAccessToken(user.ID, org.ID, user.Email, user.Name, user.Role)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not issue session")
		return
	}

	refreshPlain, refreshHash, err := auth.GenerateOpaqueToken()
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not issue refresh token")
		return
	}
	csrfPlain, _, err := auth.GenerateOpaqueToken()
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not issue CSRF token")
		return
	}

	if _, err := a.store.CreateRefreshSession(r.Context(), user.ID, refreshHash, time.Now().UTC().Add(a.tokens.RefreshTTL())); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not store refresh session")
		return
	}

	a.setSessionCookies(w, refreshPlain, csrfPlain, true)
	a.audit.Record(r.Context(), org.ID, &user.ID, "user.registered", "user", user.ID.String(), middleware.ClientIP(r), map[string]any{
		"email":             user.Email,
		"organization_name": org.Name,
	})

	response.JSON(w, r, http.StatusCreated, map[string]any{
		"access_token": accessToken,
		"expires_at":   exp,
		"csrf_token":   csrfPlain,
		"user":         user,
		"organization": org,
	}, "Account registered successfully")
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		RememberMe bool   `json:"remember_me"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || req.Password == "" {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Email and password are required")
		return
	}

	user, err := a.store.GetUserByEmail(r.Context(), email)
	if err != nil || !user.IsActive || !auth.VerifyPassword(user.PasswordHash, req.Password) {
		if err == nil {
			a.audit.Record(r.Context(), user.OrganizationID, &user.ID, "auth.login_failed", "user", user.ID.String(), middleware.ClientIP(r), map[string]any{
				"reason": "invalid_credentials_or_inactive",
			})
		}
		// Generic error message to avoid revealing whether an account exists (PRD 24.1)
		response.Error(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
		return
	}

	org, err := a.store.GetOrganization(r.Context(), user.OrganizationID)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load organization")
		return
	}

	accessToken, exp, err := a.tokens.GenerateAccessToken(user.ID, org.ID, user.Email, user.Name, user.Role)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not issue token")
		return
	}

	refreshPlain, refreshHash, err := auth.GenerateOpaqueToken()
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not issue refresh token")
		return
	}
	csrfPlain, _, err := auth.GenerateOpaqueToken()
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not issue CSRF token")
		return
	}

	ttl := 24 * time.Hour
	if req.RememberMe {
		ttl = a.tokens.RefreshTTL()
	}
	if _, err := a.store.CreateRefreshSession(r.Context(), user.ID, refreshHash, time.Now().UTC().Add(ttl)); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not store refresh session")
		return
	}

	a.setSessionCookies(w, refreshPlain, csrfPlain, req.RememberMe)
	a.audit.Record(r.Context(), org.ID, &user.ID, "auth.login_success", "user", user.ID.String(), middleware.ClientIP(r), map[string]any{
		"remember_me": req.RememberMe,
	})

	response.JSON(w, r, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"expires_at":   exp,
		"csrf_token":   csrfPlain,
		"user":         user,
		"organization": org,
	}, "Signed in successfully")
}

func (a *API) handleRefresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		response.Error(w, r, http.StatusUnauthorized, "SESSION_EXPIRED", "Refresh session not found")
		return
	}

	oldHash := auth.HashToken(cookie.Value)
	session, err := a.store.GetRefreshSessionByHash(r.Context(), oldHash)
	if err != nil || session.RevokedAt != nil || session.ExpiresAt.Before(time.Now().UTC()) {
		a.clearSessionCookies(w)
		response.Error(w, r, http.StatusUnauthorized, "SESSION_EXPIRED", "Session has expired or been revoked")
		return
	}

	user, err := a.store.GetUserByID(r.Context(), session.UserID)
	if err != nil || !user.IsActive {
		a.clearSessionCookies(w)
		response.Error(w, r, http.StatusUnauthorized, "ACCOUNT_DISABLED", "Account is unavailable")
		return
	}

	org, err := a.store.GetOrganization(r.Context(), user.OrganizationID)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Organization not found")
		return
	}

	// Rotate refresh token
	_ = a.store.RevokeRefreshSession(r.Context(), oldHash)
	newRefresh, newHash, err := auth.GenerateOpaqueToken()
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not rotate session")
		return
	}
	newCSRF, _, err := auth.GenerateOpaqueToken()
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not rotate CSRF token")
		return
	}

	if _, err := a.store.CreateRefreshSession(r.Context(), user.ID, newHash, time.Now().UTC().Add(a.tokens.RefreshTTL())); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not persist refreshed session")
		return
	}

	accessToken, exp, err := a.tokens.GenerateAccessToken(user.ID, org.ID, user.Email, user.Name, user.Role)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not issue access token")
		return
	}

	a.setSessionCookies(w, newRefresh, newCSRF, true)
	response.JSON(w, r, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"expires_at":   exp,
		"csrf_token":   newCSRF,
		"user":         user,
		"organization": org,
	}, "Session refreshed")
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(refreshCookieName); err == nil && cookie.Value != "" {
		_ = a.store.RevokeRefreshSession(r.Context(), auth.HashToken(cookie.Value))
	}
	a.clearSessionCookies(w)
	response.JSON(w, r, http.StatusOK, map[string]bool{"logged_out": true}, "Signed out successfully")
}

func (a *API) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Email is required")
		return
	}

	var devToken string
	if user, err := a.store.GetUserByEmail(r.Context(), email); err == nil && user.IsActive {
		plain, hash, err := auth.GenerateOpaqueToken()
		if err == nil {
			_ = a.store.CreatePasswordResetToken(r.Context(), user.ID, hash, time.Now().UTC().Add(30*time.Minute))
			a.audit.Record(r.Context(), user.OrganizationID, &user.ID, "auth.password_reset_requested", "user", user.ID.String(), middleware.ClientIP(r), nil)
			if strings.EqualFold(a.cfg.AppEnv, "development") {
				devToken = plain
			}
		}
	}

	payload := map[string]any{
		"instructions": "If an account exists for that email, a password reset link has been issued.",
	}
	if devToken != "" {
		payload["dev_reset_token"] = devToken
	}
	response.JSON(w, r, http.StatusOK, payload, "Password reset processed")
}

func (a *API) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	if strings.TrimSpace(req.Token) == "" {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Reset token is required")
		return
	}
	if err := auth.ValidatePasswordStrength(req.NewPassword); err != nil {
		response.Error(w, r, http.StatusBadRequest, "WEAK_PASSWORD", err.Error())
		return
	}

	userID, err := a.store.ConsumePasswordResetToken(r.Context(), auth.HashToken(strings.TrimSpace(req.Token)))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_TOKEN", "Reset token is invalid or expired")
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not update password")
		return
	}
	if err := a.store.UpdateUserPassword(r.Context(), userID, hash); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not update password")
		return
	}
	_ = a.store.RevokeAllUserSessions(r.Context(), userID)

	if user, err := a.store.GetUserByID(r.Context(), userID); err == nil {
		a.audit.Record(r.Context(), user.OrganizationID, &user.ID, "auth.password_reset_completed", "user", user.ID.String(), middleware.ClientIP(r), nil)
	}

	response.JSON(w, r, http.StatusOK, map[string]bool{"reset": true}, "Password updated successfully")
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	user, err := a.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		response.Error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "User not found")
		return
	}
	org, err := a.store.GetOrganization(r.Context(), claims.OrganizationID)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Organization not found")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"user":         user,
		"organization": org,
	}, "Profile loaded")
}

func (a *API) handleGetCurrentOrg(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	org, err := a.store.GetOrganization(r.Context(), claims.OrganizationID)
	if err != nil {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Organization not found")
		return
	}
	response.JSON(w, r, http.StatusOK, org, "Organization loaded")
}

func (a *API) handleUpdateCurrentOrg(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	current, err := a.store.GetOrganization(r.Context(), claims.OrganizationID)
	if err != nil {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Organization not found")
		return
	}

	var req struct {
		Name                     *string `json:"name"`
		Description              *string `json:"description"`
		Timezone                 *string `json:"timezone"`
		DisplayUnits             *string `json:"display_units"`
		DateFormat               *string `json:"date_format"`
		TimeFormat               *string `json:"time_format"`
		DefaultScrapeInterval    *int    `json:"default_scrape_interval"`
		DefaultMonitoringTimeout *int    `json:"default_monitoring_timeout"`
		AgentConnectionTimeout   *int    `json:"agent_connection_timeout"`
		DiskExclusionPatterns    *string `json:"disk_exclusion_patterns"`
		DefaultEnvironment       *string `json:"default_environment"`
		OnboardingCompleted      *bool   `json:"onboarding_completed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		current.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		current.Description = strings.TrimSpace(*req.Description)
	}
	if req.Timezone != nil && strings.TrimSpace(*req.Timezone) != "" {
		current.Timezone = strings.TrimSpace(*req.Timezone)
	}
	if req.DisplayUnits != nil && (*req.DisplayUnits == "binary" || *req.DisplayUnits == "decimal") {
		current.DisplayUnits = *req.DisplayUnits
	}
	if req.DateFormat != nil && strings.TrimSpace(*req.DateFormat) != "" {
		current.DateFormat = strings.TrimSpace(*req.DateFormat)
	}
	if req.TimeFormat != nil && (*req.TimeFormat == "12h" || *req.TimeFormat == "24h") {
		current.TimeFormat = *req.TimeFormat
	}
	if req.DefaultScrapeInterval != nil && *req.DefaultScrapeInterval >= 5 && *req.DefaultScrapeInterval <= 3600 {
		current.DefaultScrapeInterval = *req.DefaultScrapeInterval
	}
	if req.DefaultMonitoringTimeout != nil && *req.DefaultMonitoringTimeout >= 1 && *req.DefaultMonitoringTimeout <= 120 {
		current.DefaultMonitoringTimeout = *req.DefaultMonitoringTimeout
	}
	if req.AgentConnectionTimeout != nil && *req.AgentConnectionTimeout >= 1 && *req.AgentConnectionTimeout <= 120 {
		current.AgentConnectionTimeout = *req.AgentConnectionTimeout
	}
	if req.DiskExclusionPatterns != nil {
		current.DiskExclusionPatterns = strings.TrimSpace(*req.DiskExclusionPatterns)
	}
	if req.DefaultEnvironment != nil && isValidEnvironment(*req.DefaultEnvironment) {
		current.DefaultEnvironment = strings.ToLower(*req.DefaultEnvironment)
	}
	if req.OnboardingCompleted != nil {
		current.OnboardingCompleted = *req.OnboardingCompleted
	}

	updated, err := a.store.UpdateOrganization(r.Context(), current)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not update organization")
		return
	}
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "organization.updated", "organization", updated.ID.String(), middleware.ClientIP(r), map[string]any{
		"name":     updated.Name,
		"timezone": updated.Timezone,
	})
	response.JSON(w, r, http.StatusOK, updated, "Organization updated")
}

func (a *API) handleListUsers(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	users, err := a.store.ListUsers(r.Context(), claims.OrganizationID)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not list users")
		return
	}
	response.JSON(w, r, http.StatusOK, users, "Users loaded")
}

func (a *API) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Role = strings.TrimSpace(strings.ToLower(req.Role))

	if req.Name == "" {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Name is required")
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Valid email is required")
		return
	}
	if req.Role != models.RoleAdmin && req.Role != models.RoleOperator && req.Role != models.RoleViewer {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Role must be admin, operator, or viewer")
		return
	}
	if err := auth.ValidatePasswordStrength(req.Password); err != nil {
		response.Error(w, r, http.StatusBadRequest, "WEAK_PASSWORD", err.Error())
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not hash password")
		return
	}
	user, err := a.store.CreateUser(r.Context(), claims.OrganizationID, req.Name, req.Email, hash, req.Role)
	if errors.Is(err, repository.ErrConflict) {
		response.Error(w, r, http.StatusConflict, "EMAIL_EXISTS", "Email already in use")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not create user")
		return
	}

	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "user.created", "user", user.ID.String(), middleware.ClientIP(r), map[string]any{
		"email": user.Email,
		"role":  user.Role,
	})
	response.JSON(w, r, http.StatusCreated, user, "User created")
}

func (a *API) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	targetID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid user ID")
		return
	}
	var req struct {
		Name     string `json:"name"`
		Role     string `json:"role"`
		IsActive *bool  `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	if req.Role != "" && req.Role != models.RoleAdmin && req.Role != models.RoleOperator && req.Role != models.RoleViewer {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid role")
		return
	}
	user, err := a.store.UpdateUser(r.Context(), claims.OrganizationID, targetID, strings.TrimSpace(req.Name), req.Role, req.IsActive)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "User not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not update user")
		return
	}
	if req.IsActive != nil && !*req.IsActive {
		_ = a.store.RevokeAllUserSessions(r.Context(), targetID)
	}
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "user.role_or_profile_updated", "user", targetID.String(), middleware.ClientIP(r), map[string]any{
		"role":      user.Role,
		"is_active": user.IsActive,
	})
	response.JSON(w, r, http.StatusOK, user, "User updated")
}

func (a *API) handleDisableUser(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	targetID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid user ID")
		return
	}
	if targetID == claims.UserID {
		response.Error(w, r, http.StatusBadRequest, "SELF_DISABLE_FORBIDDEN", "You cannot disable your own account")
		return
	}
	inactive := false
	user, err := a.store.UpdateUser(r.Context(), claims.OrganizationID, targetID, "", "", &inactive)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "User not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not disable user")
		return
	}
	_ = a.store.RevokeAllUserSessions(r.Context(), targetID)
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "user.disabled", "user", targetID.String(), middleware.ClientIP(r), nil)
	response.JSON(w, r, http.StatusOK, user, "User disabled")
}

func (a *API) handleResetUserAccess(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	targetID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid user ID")
		return
	}
	user, err := a.store.GetUserByID(r.Context(), targetID)
	if err != nil || user.OrganizationID != claims.OrganizationID {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "User not found")
		return
	}
	_ = a.store.RevokeAllUserSessions(r.Context(), targetID)
	plain, hash, err := auth.GenerateOpaqueToken()
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate reset token")
		return
	}
	if err := a.store.CreatePasswordResetToken(r.Context(), targetID, hash, time.Now().UTC().Add(30*time.Minute)); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not persist reset token")
		return
	}
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "user.access_reset", "user", targetID.String(), middleware.ClientIP(r), nil)

	data := map[string]any{"sessions_revoked": true, "reset_issued": true}
	if strings.EqualFold(a.cfg.AppEnv, "development") {
		data["dev_reset_token"] = plain
	}
	response.JSON(w, r, http.StatusOK, data, "User sessions revoked and password reset issued")
}

func (a *API) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	resourceType := strings.TrimSpace(r.URL.Query().Get("resource_type"))

	logs, total, err := a.store.ListAuditLogs(r.Context(), claims.OrganizationID, action, resourceType, page, pageSize)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load audit logs")
		return
	}
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	response.JSON(w, r, http.StatusOK, response.Paginated{
		Items:      logs,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, "Audit logs loaded")
}
