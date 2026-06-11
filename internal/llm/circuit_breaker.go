package llm

import (
	"sync"
	"time"
)

// CircuitState — состояние circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Нормальная работа
	CircuitOpen                         // Провайдер отключен
	CircuitHalfOpen                     // Пробный вызов
)

// String возвращает строковое представление состояния (для логирования).
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "CLOSED"
	case CircuitOpen:
		return "OPEN"
	case CircuitHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreaker — защита от повторных вызовов к отказавшему провайдеру.
//
// Переходы состояний:
//
//	Closed  --(N failures)--> Open
//	Open    --(cooldown)----> HalfOpen
//	HalfOpen --(success)----> Closed
//	HalfOpen --(failure)----> Open
type CircuitBreaker struct {
	state        CircuitState
	maxFailures  int
	cooldown     time.Duration
	failureCount int
	lastFailure  time.Time
	lastSuccess  time.Time
	mu           sync.RWMutex
}

// NewCircuitBreaker создаёт новый CircuitBreaker.
// maxFailures — количество последовательных ошибок до размыкания цепи.
// cooldown — время ожидания перед переходом Open → HalfOpen.
func NewCircuitBreaker(maxFailures int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:       CircuitClosed,
		maxFailures: maxFailures,
		cooldown:    cooldown,
	}
}

// Allow проверяет, можно ли выполнить вызов.
// Возвращает true, если вызов разрешён.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFailure) > cb.cooldown {
			cb.state = CircuitHalfOpen
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	}
	return false
}

// RecordSuccess записывает успешный вызов.
// Переводит HalfOpen → Closed, сбрасывает счётчик ошибок.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.lastSuccess = time.Now()
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
	}
}

// RecordFailure записывает неудачный вызов.
// Возвращает true, если цепь разомкнулась.
func (cb *CircuitBreaker) RecordFailure() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailure = time.Now()

	if cb.state == CircuitHalfOpen || cb.failureCount >= cb.maxFailures {
		cb.state = CircuitOpen
		return true
	}
	return false
}

// State возвращает текущее состояние для мониторинга.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// FailureCount возвращает количество последовательных ошибок.
func (cb *CircuitBreaker) FailureCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failureCount
}

// Reset сбрасывает circuit breaker в исходное состояние.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.failureCount = 0
}
