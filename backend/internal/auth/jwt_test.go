package auth

import (
	"strings"
	"testing"
	"time"
)

func TestIssueAndParseTokens(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	manager := NewTokenManager([]byte("secret"), 15*time.Minute, 30*24*time.Hour)
	manager.now = func() time.Time { return now }

	access, refresh, expiresIn, err := manager.Issue("user-1", "employee", "session-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if expiresIn != 900 {
		t.Fatalf("expiresIn = %d, want 900", expiresIn)
	}

	claims, err := manager.ParseAccess(access)
	if err != nil {
		t.Fatalf("ParseAccess: %v", err)
	}
	if claims.Sub != "user-1" || claims.Role != "employee" || claims.SID != "session-1" || claims.Typ != TokenTypeAccess {
		t.Fatalf("неожиданные claims: %+v", claims)
	}

	refreshClaims, err := manager.ParseRefresh(refresh)
	if err != nil {
		t.Fatalf("ParseRefresh: %v", err)
	}
	if refreshClaims.Typ != TokenTypeRefresh {
		t.Fatalf("тип refresh-токена: %s", refreshClaims.Typ)
	}

	if _, err := manager.ParseAccess(refresh); err == nil {
		t.Fatal("refresh-токен не должен приниматься как access")
	}
}

func TestExpiredToken(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	manager := NewTokenManager([]byte("secret"), time.Minute, time.Hour)
	manager.now = func() time.Time { return now }

	access, _, _, err := manager.Issue("user-1", "admin", "session-1")
	if err != nil {
		t.Fatal(err)
	}

	manager.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := manager.ParseAccess(access); err != ErrExpiredToken {
		t.Fatalf("ожидалась ErrExpiredToken, получено %v", err)
	}
}

func TestTamperedToken(t *testing.T) {
	manager := NewTokenManager([]byte("secret"), time.Minute, time.Hour)
	access, _, _, err := manager.Issue("user-1", "admin", "session-1")
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.Split(access, ".")
	parts[1] = strings.Repeat("A", len(parts[1]))
	if _, err := manager.ParseAccess(strings.Join(parts, ".")); err == nil {
		t.Fatal("изменённый токен не должен проходить проверку")
	}

	other := NewTokenManager([]byte("other-secret"), time.Minute, time.Hour)
	if _, err := other.ParseAccess(access); err != ErrInvalidToken {
		t.Fatalf("ожидалась ErrInvalidToken, получено %v", err)
	}
}

func TestHashToken(t *testing.T) {
	first := HashToken("token")
	second := HashToken("token")
	if first != second || len(first) != 64 {
		t.Fatalf("HashToken недетерминирован или неверной длины: %q", first)
	}
	if HashToken("other") == first {
		t.Fatal("разные токены не должны давать одинаковый хэш")
	}
}
