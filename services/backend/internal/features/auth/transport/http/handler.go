package authhttp

import (
	"encoding/json"
	"errors"
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
	DisplayName *string   `json:"display_name" extensions:"x-nullable"`
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

type RegisterRequest struct {
	Email       string          `json:"email" format:"email" maxLength:"254" binding:"required"`
	Password    string          `json:"password" format:"password" minLength:"15" maxLength:"128" binding:"required"`
	DisplayName json.RawMessage `json:"display_name,omitempty" binding:"optional" swaggertype:"string" minLength:"1" maxLength:"200"`
}

type LoginRequest struct {
	Email    string `json:"email" format:"email" maxLength:"254" binding:"required"`
	Password string `json:"password" format:"password" minLength:"1" maxLength:"128" binding:"required"`
}

type authSession struct {
	User    wireUser   `json:"user"`
	Session wireExpiry `json:"session"`
}

// Register handles POST /auth/register.
// @Summary Зарегистрироваться
// @ID register
// @Tags Auth
// @Produce json
// @Accept json
// @Param request body RegisterRequest true "Тело запроса"
// @Success 201 {object} authSession
// @Header 201 {string} Set-Cookie "access_token и refresh_token: Path=/; Secure; HttpOnly; SameSite=Lax"
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /auth/register [post]
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	var name *string
	if req.DisplayName != nil {
		var n string
		if string(req.DisplayName) == "null" || json.Unmarshal(req.DisplayName, &n) != nil {
			httpx.Error(r.Context(), w, fault.Validation("display_name", "Имя должно содержать 1–200 символов."))
			return
		}
		name = &n
	}
	result, err := h.service.Register(r.Context(), application.RegisterInput{Email: req.Email, Password: req.Password, DisplayName: name})
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	setCookies(w, result.Tokens, h.now().Unix())
	httpx.JSON(w, 201, authSession{userDTO(result.User), expiryDTO(result.Tokens.Expiry)})
}

// Login handles POST /auth/login.
// @Summary Войти
// @ID login
// @Tags Auth
// @Produce json
// @Accept json
// @Param request body LoginRequest true "Тело запроса"
// @Success 200 {object} authSession
// @Header 200 {string} Set-Cookie "access_token и refresh_token: Path=/; Secure; HttpOnly; SameSite=Lax"
// @Failure 401 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Router /auth/login [post]
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	result, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	setCookies(w, result.Tokens, h.now().Unix())
	httpx.JSON(w, 200, authSession{userDTO(result.User), expiryDTO(result.Tokens.Expiry)})
}

// Refresh handles POST /auth/refresh.
// @Summary Обновить токены сессии
// @ID refreshTokens
// @Tags Auth
// @Produce json
// @Security refreshCookie
// @Success 200 {object} wireExpiry
// @Header 200 {string} Set-Cookie "access_token и refresh_token: Path=/; Secure; HttpOnly; SameSite=Lax"
// @Failure 401 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Router /auth/refresh [post]
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	if err := httpx.NoBody(r); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	t, err := h.service.Refresh(r.Context(), httpx.Cookie(r, "refresh_token"))
	if err != nil {
		var e *fault.Error
		if errors.As(err, &e) && e.Code == "INVALID_REFRESH_TOKEN" {
			clearCookies(w)
		}
		httpx.Error(r.Context(), w, err)
		return
	}
	setCookies(w, t, h.now().Unix())
	httpx.JSON(w, 200, expiryDTO(t.Expiry))
}

// Logout handles POST /auth/logout.
// @Summary Завершить сессию
// @ID logout
// @Tags Auth
// @Produce json
// @Description Refresh cookie необязательна; без неё возвращается 204 и удаляются обе cookies.
// @Success 204 "Сессия завершена"
// @Header 204 {string} Set-Cookie "access_token и refresh_token: Path=/; Secure; HttpOnly; SameSite=Lax"
// @Failure 422 {object} fault.Error
// @Router /auth/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := httpx.NoBody(r); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	if err := h.service.Logout(r.Context(), httpx.Cookie(r, "refresh_token")); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	clearCookies(w)
	w.WriteHeader(204)
}

// Me handles GET /auth/me.
// @Summary Получить текущего пользователя
// @ID getCurrentUser
// @Tags Auth
// @Produce json
// @Security accessCookie
// @Success 200 {object} wireUser
// @Failure 401 {object} fault.Error
// @Router /auth/me [get]
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	u, err := h.service.Me(r.Context(), httpx.Cookie(r, "access_token"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
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
