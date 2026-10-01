package authhttp

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/session"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type Handler struct {
	service *application.Service
	now     func() time.Time
}

func New(service *application.Service, now func() time.Time) *Handler { return &Handler{service, now} }

type wireUser struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName *string   `json:"display_name"`
	Role        user.Role `json:"role"`
	CreatedAt   int64     `json:"created_at"`
}
type wireExpiry struct {
	AccessExpiresAt  int64 `json:"access_expires_at"`
	RefreshExpiresAt int64 `json:"refresh_expires_at"`
}

func userDTO(u user.User) wireUser {
	return wireUser{u.ID, u.Email, u.DisplayName, u.Role, u.CreatedAt}
}
func expiryDTO(e session.Expiry) wireExpiry { return wireExpiry{e.AccessExpiresAt, e.RefreshExpiresAt} }

type authSession struct {
	User    wireUser   `json:"user"`
	Session wireExpiry `json:"session"`
}

func (h *Handler) rate(w http.ResponseWriter, r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if err = h.service.RateIP(r.Context(), ip); err != nil {
		httpx.Error(w, err)
		return false
	}
	return true
}
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if !h.rate(w, r) {
		return
	}
	var req struct {
		Email       string          `json:"email"`
		Password    string          `json:"password"`
		DisplayName json.RawMessage `json:"display_name,omitempty"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	var name *string
	if req.DisplayName != nil {
		var n string
		if string(req.DisplayName) == "null" || json.Unmarshal(req.DisplayName, &n) != nil {
			httpx.Error(w, fault.Validation("display_name", "Имя должно содержать 1–200 символов."))
			return
		}
		name = &n
	}
	result, err := h.service.Register(r.Context(), application.RegisterInput{Email: req.Email, Password: req.Password, DisplayName: name})
	if err != nil {
		httpx.Error(w, err)
		return
	}
	setCookies(w, result.Tokens, h.now().Unix())
	httpx.JSON(w, 201, authSession{userDTO(result.User), expiryDTO(result.Tokens.Expiry)})
}
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.rate(w, r) {
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	result, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	setCookies(w, result.Tokens, h.now().Unix())
	httpx.JSON(w, 200, authSession{userDTO(result.User), expiryDTO(result.Tokens.Expiry)})
}
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	if !h.rate(w, r) {
		return
	}
	if err := httpx.NoBody(r); err != nil {
		httpx.Error(w, err)
		return
	}
	t, err := h.service.Refresh(r.Context(), httpx.Cookie(r, "refresh_token"))
	if err != nil {
		var e *fault.Error
		if errors.As(err, &e) && e.Code == "INVALID_REFRESH_TOKEN" {
			clearCookies(w)
		}
		httpx.Error(w, err)
		return
	}
	setCookies(w, t, h.now().Unix())
	httpx.JSON(w, 200, expiryDTO(t.Expiry))
}
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if !h.rate(w, r) {
		return
	}
	if err := httpx.NoBody(r); err != nil {
		httpx.Error(w, err)
		return
	}
	if err := h.service.Logout(r.Context(), httpx.Cookie(r, "refresh_token")); err != nil {
		httpx.Error(w, err)
		return
	}
	clearCookies(w)
	w.WriteHeader(204)
}
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	u, err := h.service.Me(r.Context(), httpx.Cookie(r, "access_token"))
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, 200, userDTO(u))
}
func setCookies(w http.ResponseWriter, t session.Tokens, now int64) {
	for _, c := range []*http.Cookie{{Name: "access_token", Value: t.Access, Path: "/", MaxAge: int(t.Expiry.AccessExpiresAt - now)}, {Name: "refresh_token", Value: t.Refresh, Path: "/", MaxAge: int(t.Expiry.RefreshExpiresAt - now)}} {
		c.Secure = true
		c.HttpOnly = true
		c.SameSite = http.SameSiteLaxMode
		http.SetCookie(w, c)
	}
}
func clearCookies(w http.ResponseWriter) {
	for _, c := range []*http.Cookie{{Name: "access_token", Path: "/", MaxAge: -1}, {Name: "refresh_token", Path: "/", MaxAge: -1}} {
		c.Secure = true
		c.HttpOnly = true
		c.SameSite = http.SameSiteLaxMode
		http.SetCookie(w, c)
	}
}
