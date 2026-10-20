package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

const (
	passwordAlgo = "argon2id"
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
	maxPassword  = 1024
)

var ErrInvalidPassword = errors.New("неверный пароль")

// ValidatePassword проверяет политику пароля: не менее 5 символов,
// хотя бы одна буква и хотя бы одна цифра.
func ValidatePassword(password string) error {
	if len(password) > maxPassword {
		return fmt.Errorf("пароль слишком длинный (максимум %d символов)", maxPassword)
	}
	runes := []rune(password)
	if len(runes) < 5 {
		return errors.New("пароль должен содержать не менее 5 символов")
	}
	var hasLetter, hasDigit bool
	for _, r := range runes {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errors.New("пароль должен содержать букву и цифру")
	}
	return nil
}

// HashPassword возвращает хэш пароля в формате PHC (argon2id).
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("генерация соли: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		passwordAlgo,
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword сверяет пароль с PHC-хэшем argon2id.
func VerifyPassword(encoded, password string) (bool, error) {
	params, salt, expected, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, params.keyLen)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
	keyLen  uint32
}

func decodeHash(encoded string) (argonParams, []byte, []byte, error) {
	var params argonParams
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" {
		return params, nil, nil, errors.New("некорректный формат хэша пароля")
	}
	if parts[1] != passwordAlgo {
		return params, nil, nil, fmt.Errorf("неподдерживаемый алгоритм хэширования %q", parts[1])
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return params, nil, nil, fmt.Errorf("версия argon2: %w", err)
	}
	if version != argon2.Version {
		return params, nil, nil, fmt.Errorf("неподдерживаемая версия argon2: %d", version)
	}

	var memory, timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return params, nil, nil, fmt.Errorf("параметры argon2: %w", err)
	}
	if memory == 0 || timeCost == 0 || threads == 0 {
		return params, nil, nil, errors.New("некорректные параметры argon2")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return params, nil, nil, fmt.Errorf("соль: %w", err)
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return params, nil, nil, fmt.Errorf("хэш: %w", err)
	}
	if len(salt) == 0 || len(hash) == 0 {
		return params, nil, nil, errors.New("пустая соль или хэш")
	}
	if len(hash) > 1024 {
		return params, nil, nil, errors.New("слишком длинный хэш")
	}

	params.memory = memory
	params.time = timeCost
	params.threads = threads
	params.keyLen = uint32(len(hash))
	return params, salt, hash, nil
}
