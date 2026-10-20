package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"tabel/internal/repo"
)

type OAuthPrincipal struct {
	ClientUUID string
	ClientID   string
}

type oauthPrincipalKey struct{}

func oauthPrincipalFrom(ctx context.Context) (OAuthPrincipal, bool) {
	p, ok := ctx.Value(oauthPrincipalKey{}).(OAuthPrincipal)
	return p, ok
}

func (s *Server) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "некорректная форма")
		return
	}
	if r.PostFormValue("grant_type") != "client_credentials" {
		writeError(w, http.StatusBadRequest, "grant_type должен быть client_credentials")
		return
	}
	clientID := strings.TrimSpace(r.PostFormValue("client_id"))
	clientSecret := r.PostFormValue("client_secret")
	if clientID == "" || clientSecret == "" {
		writeError(w, http.StatusBadRequest, "client_id и client_secret обязательны")
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	client, err := rp.AuthenticateOAuthClient(r.Context(), clientID, clientSecret)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "неверные client_id или client_secret")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка аутентификации клиента")
		return
	}
	allowed, err := s.oauthIPAllowed(r, rp, client.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка проверки IP")
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "IP-адрес не входит в whitelist")
		return
	}
	token, err := repo.RandomURLToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка генерации токена")
		return
	}
	expiresIn := 3600
	expiresAt := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second)
	if err := rp.CreateOAuthAccessToken(r.Context(), client.ClientID, repo.HashOAuthSecret(token), expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить токен")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   expiresIn,
	})
}

func (s *Server) oauthIPAllowed(r *http.Request, rp *repo.Repo, clientUUID string) (bool, error) {
	ipValue := clientIP(r)
	if ipValue == nil {
		return false, nil
	}
	ip := net.ParseIP(*ipValue)
	if ip == nil {
		return false, nil
	}
	return rp.IPAllowed(r.Context(), clientUUID, ip)
}

func (s *Server) oauth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "требуется OAuth-токен")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		rp, ok := s.repo()
		if !ok {
			writeError(w, http.StatusServiceUnavailable, "система не настроена")
			return
		}
		access, err := rp.FindOAuthAccessToken(r.Context(), repo.HashOAuthSecret(token))
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "недействительный или истёкший токен")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ошибка проверки токена")
			return
		}
		allowed, err := s.oauthIPAllowed(r, rp, access.ClientUUID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ошибка проверки IP")
			return
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "IP-адрес не входит в whitelist")
			return
		}
		principal := OAuthPrincipal{ClientUUID: access.ClientUUID, ClientID: access.ClientID}
		next(w, r.WithContext(context.WithValue(r.Context(), oauthPrincipalKey{}, principal)))
	}
}

func (s *Server) handleListOAuthClients(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	clients, err := rp.ListOAuthClients(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить список клиентов")
		return
	}
	if clients == nil {
		clients = []repo.OAuthClient{}
	}
	writeJSON(w, http.StatusOK, clients)
}

func (s *Server) handleCreateOAuthClient(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "поле name обязательно")
		return
	}
	client, err := rp.CreateOAuthClient(r.Context(), req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать клиента")
		return
	}
	principal, _ := principalFrom(r.Context())
	s.audit(r, rp, &principal.UserID, "oauth.client_create", "oauth_client", client.ID, map[string]any{"client_id": client.ClientID, "name": req.Name})
	writeJSON(w, http.StatusCreated, client)
}

func (s *Server) handleRotateOAuthSecret(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	client, err := rp.RotateOAuthSecret(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "клиент не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось ротировать секрет")
		return
	}
	principal, _ := principalFrom(r.Context())
	s.audit(r, rp, &principal.UserID, "oauth.client_rotate", "oauth_client", client.ID, map[string]any{"client_id": client.ClientID})
	writeJSON(w, http.StatusOK, client)
}

func (s *Server) handleListIPWhitelist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	entries, err := rp.ListIPWhitelist(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить whitelist")
		return
	}
	if entries == nil {
		entries = []repo.IpWhitelistEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleAddIPWhitelist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	if _, err := rp.OAuthClientByUUID(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "клиент не найден")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения клиента")
		return
	}
	var req struct {
		CIDR        string `json:"cidr"`
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.CIDR = strings.TrimSpace(req.CIDR)
	if _, _, err := net.ParseCIDR(req.CIDR); err != nil {
		writeError(w, http.StatusBadRequest, "cidr должен быть корректным CIDR (например 203.0.113.0/24)")
		return
	}
	var description *string
	if strings.TrimSpace(req.Description) != "" {
		value := strings.TrimSpace(req.Description)
		description = &value
	}
	entry, err := rp.AddIPWhitelist(r.Context(), id, req.CIDR, description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось добавить CIDR")
		return
	}
	principal, _ := principalFrom(r.Context())
	s.audit(r, rp, &principal.UserID, "oauth.whitelist_add", "oauth_client", id, map[string]any{"cidr": req.CIDR})
	writeJSON(w, http.StatusOK, entry)
}
