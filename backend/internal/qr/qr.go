// Package qr реализует криптографию QR-токенов: каноническое представление
// полезной нагрузки и подпись/проверку ECDSA P-256.
package qr

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"sync"

	"tabel/internal/settings"
)

var ErrNoKey = errors.New("ключ QR не найден")

// Manager хранит приватный ключ QR в SQLite-хранилище настроек (зашифрованным).
// Ключ генерируется при первом обращении, если он ещё не создан.
type Manager struct {
	store *settings.Store

	mu   sync.Mutex
	priv *ecdsa.PrivateKey
}

func NewManager(store *settings.Store) *Manager {
	return &Manager{store: store}
}

// CanonicalJSON приводит JSON к детерминированному виду (сортировка ключей,
// без пробелов, числа сохраняются как json.Number). Верхнеуровневое поле
// "signature" исключается: подписывается только полезная нагрузка.
func CanonicalJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("разбор JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("лишние данные после JSON")
		}
		return nil, fmt.Errorf("разбор JSON: %w", err)
	}
	if object, ok := value.(map[string]any); ok {
		delete(object, "signature")
	}
	out, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("каноническая сериализация: %w", err)
	}
	return out, nil
}

func (m *Manager) privateKey() (*ecdsa.PrivateKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.priv != nil {
		return m.priv, nil
	}

	if encoded, err := m.store.GetString(settings.KeyQRPrivateKey); err == nil && encoded != "" {
		key, err := parsePrivateKey([]byte(encoded))
		if err != nil {
			return nil, err
		}
		m.priv = key
		return m.priv, nil
	} else if err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, fmt.Errorf("чтение ключа QR: %w", err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("генерация ключа QR: %w", err)
	}
	encoded, err := encodePrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := m.store.SetString(settings.KeyQRPrivateKey, string(encoded), true); err != nil {
		return nil, fmt.Errorf("сохранение ключа QR: %w", err)
	}
	m.priv = key
	return m.priv, nil
}

// PublicKeyPEM возвращает публичный ключ в формате PEM (SubjectPublicKeyInfo).
func (m *Manager) PublicKeyPEM() (string, error) {
	key, err := m.privateKey()
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", fmt.Errorf("сериализация публичного ключа: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}

// Sign подписывает каноническую JSON-полезную нагрузку (ECDSA P-256, SHA-256).
func (m *Manager) Sign(payload []byte) (string, error) {
	key, err := m.privateKey()
	if err != nil {
		return "", err
	}
	canonical, err := CanonicalJSON(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		return "", fmt.Errorf("подпись QR: %w", err)
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// Verify проверяет подпись полезной нагрузки публичным ключом сервера.
func (m *Manager) Verify(payload []byte, signature string) (bool, error) {
	if signature == "" {
		return false, nil
	}
	key, err := m.privateKey()
	if err != nil {
		return false, err
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false, nil
	}
	canonical, err := CanonicalJSON(payload)
	if err != nil {
		return false, err
	}
	digest := sha256.Sum256(canonical)
	return ecdsa.VerifyASN1(&key.PublicKey, digest[:], sig), nil
}

func encodePrivateKey(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("сериализация ключа QR: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func parsePrivateKey(encoded []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(encoded)
	if block == nil {
		return nil, errors.New("некорректный PEM ключа QR")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("разбор ключа QR: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("ключ QR должен быть ECDSA P-256")
	}
	return key, nil
}
