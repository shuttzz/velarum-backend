package auth

import (
	"context"
	"net/http"
	"time"
)

type ctxKey int

const accountKey ctxKey = 0

// WithAccount injeta o ID da conta autenticada no contexto.
func WithAccount(ctx context.Context, accountID string) context.Context {
	return context.WithValue(ctx, accountKey, accountID)
}

// AccountID lê o ID da conta autenticada do contexto.
func AccountID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(accountKey).(string)
	return id, ok && id != ""
}

// Require é um middleware que exige sessão válida (cookie). Em sucesso, injeta o accountID
// no contexto e segue; senão responde 401.
func (s *Service) Require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID, err := s.Authenticate(r.Context(), readCookie(r), time.Now().UTC())
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"unauthenticated","error":"não autenticado"}`))
			return
		}
		next(w, r.WithContext(WithAccount(r.Context(), accountID)))
	}
}

func readCookie(r *http.Request) string {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// SetSessionCookie escreve o cookie de sessão (httpOnly, SameSite=Strict).
func (s *Service) SetSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearSessionCookie expira o cookie de sessão no cliente (logout).
func (s *Service) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}
