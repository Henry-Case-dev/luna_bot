package llm

import (
	"strings"
	"sync"
	"time"
)

// KeyRotator — middleware для ротации API-ключей.
//
// Используется провайдерами для автоматического переключения
// на резервный ключ при ошибках 429/5xx и возврата на основной
// ключ после истечения TTL.
type KeyRotator struct {
	primaryKey   string
	reserveKey   string
	rotationTTL  time.Duration
	lastSwitch   time.Time
	usingReserve bool
	mu           sync.Mutex
}

// NewKeyRotator создаёт новый KeyRotator.
// primaryKey — основной API-ключ.
// reserveKey — резервный API-ключ (может быть пустым).
// rotationTTL — время, через которое происходит попытка возврата на основной ключ.
func NewKeyRotator(primaryKey, reserveKey string, rotationTTL time.Duration) *KeyRotator {
	return &KeyRotator{
		primaryKey:  primaryKey,
		reserveKey:  reserveKey,
		rotationTTL: rotationTTL,
	}
}

// GetKey возвращает текущий активный ключ.
// Если используется резервный ключ и TTL истёк — возвращает на primary.
func (kr *KeyRotator) GetKey() string {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	// Пытаемся вернуться на основной ключ после TTL
	if kr.usingReserve && time.Since(kr.lastSwitch) > kr.rotationTTL {
		kr.primaryKey, kr.reserveKey = kr.reserveKey, kr.primaryKey
		kr.usingReserve = false
		kr.lastSwitch = time.Now()
	}

	return kr.primaryKey
}

// RotateOnError проверяет ошибку и при необходимости переключает ключ.
// При ошибках 429/5xx меняет primary ↔ reserve местами.
// Возвращает true, если ключ был переключён.
func (kr *KeyRotator) RotateOnError(err error) bool {
	if !isRateLimitOrServerError(err) {
		return false
	}

	kr.mu.Lock()
	defer kr.mu.Unlock()

	if kr.reserveKey == "" {
		return false
	}

	kr.primaryKey, kr.reserveKey = kr.reserveKey, kr.primaryKey
	kr.usingReserve = !kr.usingReserve
	kr.lastSwitch = time.Now()
	return true
}

// UsingReserve возвращает true, если сейчас используется резервный ключ.
func (kr *KeyRotator) UsingReserve() bool {
	kr.mu.Lock()
	defer kr.mu.Unlock()
	return kr.usingReserve
}

// isRateLimitOrServerError проверяет, является ли ошибка связанной с
// превышением лимита (429) или серверной ошибкой (5xx).
// Проверяет HTTP-коды в строке ошибки.
func isRateLimitOrServerError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504")
}
