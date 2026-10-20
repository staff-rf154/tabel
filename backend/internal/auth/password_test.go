package auth

import (
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"корректный", "pass1", false},
		{"ровно 5 символов", "a1234", false},
		{"короче 5", "a123", true},
		{"без цифры", "password", true},
		{"без буквы", "123456", true},
		{"кириллица и цифра", "пароль1", false},
		{"пустой", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePassword(tt.password); (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePassword(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("Secret1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("неожиданный формат хэша: %s", hash)
	}

	ok, err := VerifyPassword(hash, "Secret1")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("пароль должен совпадать")
	}

	ok, err = VerifyPassword(hash, "Secret2")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Fatal("неверный пароль не должен совпадать")
	}
}

func TestHashPasswordUniqueSalt(t *testing.T) {
	first, err := HashPassword("Secret1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword("Secret1")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("хэши одного пароля не должны совпадать из-за случайной соли")
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	for _, hash := range []string{"", "plain", "$argon2id$v=19$m=65536,t=3,p=4$!!!$@@@", "$argon2id$v=19$m=0,t=0,p=0$c2FsdA$aGFzaA", "$bcrypt$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA"} {
		if _, err := VerifyPassword(hash, "Secret1"); err == nil {
			t.Fatalf("ожидалась ошибка для хэша %q", hash)
		}
	}
}
