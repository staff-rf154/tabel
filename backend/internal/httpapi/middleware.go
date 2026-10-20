package httpapi

import (
	"context"
	"net"
	"net/http"
	"strings"

	"tabel/internal/settings"
)

type Principal struct {
	UserID    string
	Role      string
	SessionID string
}

type principalKey struct{}

func principalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

func (s *Server) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "требуется авторизация")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		claims, err := s.tokens.ParseAccess(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "недействительный или истёкший токен")
			return
		}
		p := Principal{UserID: claims.Sub, Role: claims.Role, SessionID: claims.SID}
		next(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	}
}

func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return s.authenticated(func(w http.ResponseWriter, r *http.Request) {
		p, _ := principalFrom(r.Context())
		if p.Role != "admin" {
			writeError(w, http.StatusForbidden, "недостаточно прав")
			return
		}
		next(w, r)
	})
}

func (s *Server) writeBlocked(w http.ResponseWriter, message string) {
	phone, _ := s.store.GetString(settings.KeySupportPhone)
	writeJSON(w, http.StatusLocked, map[string]string{
		"error":         message,
		"support_phone": phone,
	})
}

func clientIP(r *http.Request) *string {
	if value := strings.TrimSpace(r.Header.Get("X-Real-IP")); value != "" {
		if ip := net.ParseIP(value); ip != nil {
			return &value
		}
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
		if ip := net.ParseIP(first); ip != nil {
			return &first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return &host
	}
	return nil
}

func userAgent(r *http.Request) string {
	return strings.TrimSpace(r.UserAgent())
}
