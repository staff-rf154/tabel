package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"

	jwtHeader = `{"alg":"HS256","typ":"JWT"}`
)

var (
	ErrInvalidToken = errors.New("недействительный токен")
	ErrExpiredToken = errors.New("срок действия токена истёк")
)

type Claims struct {
	Sub  string `json:"sub"`
	Role string `json:"role"`
	SID  string `json:"sid"`
	Typ  string `json:"typ"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
}

type TokenManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

func NewTokenManager(secret []byte, accessTTL, refreshTTL time.Duration) *TokenManager {
	if accessTTL <= 0 {
		accessTTL = 15 * time.Minute
	}
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	return &TokenManager{
		secret:     secret,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		now:        time.Now,
	}
}

// Issue возвращает access- и refresh-токены для пользователя и его сессии.
func (m *TokenManager) Issue(userID, role, sessionID string) (accessToken, refreshToken string, expiresIn int, err error) {
	now := m.now()
	access, err := m.sign(Claims{
		Sub:  userID,
		Role: role,
		SID:  sessionID,
		Typ:  TokenTypeAccess,
		Iat:  now.Unix(),
		Exp:  now.Add(m.accessTTL).Unix(),
	})
	if err != nil {
		return "", "", 0, err
	}
	refresh, err := m.sign(Claims{
		Sub:  userID,
		Role: role,
		SID:  sessionID,
		Typ:  TokenTypeRefresh,
		Iat:  now.Unix(),
		Exp:  now.Add(m.refreshTTL).Unix(),
	})
	if err != nil {
		return "", "", 0, err
	}
	return access, refresh, int(m.accessTTL.Seconds()), nil
}

func (m *TokenManager) ParseAccess(token string) (Claims, error) {
	return m.parse(token, TokenTypeAccess)
}

func (m *TokenManager) ParseRefresh(token string) (Claims, error) {
	return m.parse(token, TokenTypeRefresh)
}

func (m *TokenManager) RefreshTTL() time.Duration {
	return m.refreshTTL
}

func (m *TokenManager) sign(claims Claims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("сериализация токена: %w", err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(jwtHeader))
	body := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := header + "." + body
	signature := m.signature(signingInput)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (m *TokenManager) parse(token, expectedType string) (Claims, error) {
	var claims Claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, ErrInvalidToken
	}

	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || string(headerRaw) != jwtHeader {
		return claims, ErrInvalidToken
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims, ErrInvalidToken
	}
	if !hmac.Equal(signature, m.signature(parts[0]+"."+parts[1])) {
		return claims, ErrInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, ErrInvalidToken
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, ErrInvalidToken
	}
	if claims.Sub == "" || claims.Typ == "" {
		return claims, ErrInvalidToken
	}
	if claims.Typ != expectedType {
		return claims, ErrInvalidToken
	}
	if claims.Exp == 0 || m.now().Unix() >= claims.Exp {
		return claims, ErrExpiredToken
	}
	return claims, nil
}

func (m *TokenManager) signature(input string) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(input))
	return mac.Sum(nil)
}

// HashToken возвращает детерминированный хэш токена для хранения в БД.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
