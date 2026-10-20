package qr

import (
	"crypto/x509"
	"encoding/pem"
	"path/filepath"
	"testing"

	"tabel/internal/settings"
)

func newManager(t *testing.T) *Manager {
	t.Helper()
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.db"), make([]byte, 32))
	if err != nil {
		t.Fatalf("settings.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewManager(store)
}

func TestCanonicalJSON(t *testing.T) {
	input := []byte(`{"b": 1, "a": {"z": true, "c": [3, 2, 1]}}`)
	got, err := CanonicalJSON(input)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	want := `{"a":{"c":[3,2,1],"z":true},"b":1}`
	if string(got) != want {
		t.Fatalf("CanonicalJSON = %s, want %s", got, want)
	}

	if _, err := CanonicalJSON([]byte(`{"a":`)); err == nil {
		t.Fatal("ожидалась ошибка для некорректного JSON")
	}
}

func TestSignAndVerify(t *testing.T) {
	manager := newManager(t)
	payload := []byte(`{"employee_number":"E-1","full_name":"Иванов Иван"}`)

	signature, err := manager.Sign(payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	valid, err := manager.Verify(payload, signature)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Fatal("подпись должна быть валидной")
	}

	reordered := []byte(`{"full_name":"Иванов Иван","employee_number":"E-1"}`)
	valid, err = manager.Verify(reordered, signature)
	if err != nil {
		t.Fatalf("Verify reordered: %v", err)
	}
	if !valid {
		t.Fatal("подпись не должна зависеть от порядка ключей JSON")
	}

	tampered := []byte(`{"employee_number":"E-2","full_name":"Иванов Иван"}`)
	valid, err = manager.Verify(tampered, signature)
	if err != nil {
		t.Fatalf("Verify tampered: %v", err)
	}
	if valid {
		t.Fatal("подпись изменённых данных не должна быть валидной")
	}

	valid, err = manager.Verify(payload, "")
	if err != nil {
		t.Fatalf("Verify empty: %v", err)
	}
	if valid {
		t.Fatal("пустая подпись не должна считаться валидной")
	}
}

func TestKeyPersistedAndPublicKey(t *testing.T) {
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.db"), make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	first := NewManager(store)
	pemKey, err := first.PublicKeyPEM()
	if err != nil {
		t.Fatalf("PublicKeyPEM: %v", err)
	}
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		t.Fatal("публичный ключ не в PEM")
	}
	if _, err := x509.ParsePKIXPublicKey(block.Bytes); err != nil {
		t.Fatalf("ParsePKIXPublicKey: %v", err)
	}

	signature, err := first.Sign([]byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}

	// Ключ должен сохраняться в SQLite и переиспользоваться после перезапуска.
	second := NewManager(store)
	valid, err := second.Verify([]byte(`{"a":1}`), signature)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("ключ должен сохраняться между экземплярами менеджера")
	}
}

func TestSignatureFieldExcluded(t *testing.T) {
	manager := newManager(t)
	withSignature := []byte(`{"employee_number":"E-1","signature":"client-value"}`)
	signature, err := manager.Sign(withSignature)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	withoutSignature := []byte(`{"employee_number":"E-1"}`)
	valid, err := manager.Verify(withoutSignature, signature)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Fatal("поле signature не должно участвовать в подписи")
	}

	tamperedSignatureField := []byte(`{"employee_number":"E-1","signature":"other"}`)
	valid, err = manager.Verify(tamperedSignatureField, signature)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Fatal("значение поля signature не должно влиять на проверку")
	}
}

func TestVerifyMalformedSignature(t *testing.T) {
	manager := newManager(t)
	valid, err := manager.Verify([]byte(`{"a":1}`), "not-base64")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if valid {
		t.Fatal("некорректная подпись не должна считаться валидной")
	}
}
