package llm

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestKeyRotator_RotateOn429(t *testing.T) {
	kr := NewKeyRotator("primary-key", "reserve-key", time.Hour)

	if kr.UsingReserve() {
		t.Fatal("should not use reserve initially")
	}
	if kr.GetKey() != "primary-key" {
		t.Fatal("initial key should be primary")
	}

	rotated := kr.RotateOnError(fmt.Errorf("HTTP 429 Too Many Requests"))
	if !rotated {
		t.Fatal("expected rotation on 429 error")
	}
	if !kr.UsingReserve() {
		t.Fatal("should use reserve after rotation on 429")
	}
	if kr.GetKey() != "reserve-key" {
		t.Fatalf("expected reserve-key after rotation, got %s", kr.GetKey())
	}
}

func TestKeyRotator_RotateOn503(t *testing.T) {
	kr := NewKeyRotator("primary-key", "reserve-key", time.Hour)

	rotated := kr.RotateOnError(fmt.Errorf("server error: 503 Service Unavailable"))
	if !rotated {
		t.Fatal("expected rotation on 503 error")
	}
	if !kr.UsingReserve() {
		t.Fatal("should use reserve after rotation on 503")
	}
	if kr.GetKey() != "reserve-key" {
		t.Fatalf("expected reserve-key after rotation, got %s", kr.GetKey())
	}
}

func TestKeyRotator_NoRotateOn400(t *testing.T) {
	kr := NewKeyRotator("primary-key", "reserve-key", time.Hour)

	rotated := kr.RotateOnError(fmt.Errorf("HTTP 400 Bad Request"))
	if rotated {
		t.Fatal("should not rotate on 400 error")
	}
	if kr.UsingReserve() {
		t.Fatal("should not use reserve after 400")
	}
	if kr.GetKey() != "primary-key" {
		t.Fatal("key should remain primary after 400")
	}
}

func TestKeyRotator_NoRotateOnNilError(t *testing.T) {
	kr := NewKeyRotator("primary-key", "reserve-key", time.Hour)

	rotated := kr.RotateOnError(nil)
	if rotated {
		t.Fatal("should not rotate on nil error")
	}
	if kr.UsingReserve() {
		t.Fatal("should not use reserve after nil error")
	}
	if kr.GetKey() != "primary-key" {
		t.Fatal("key should remain primary after nil error")
	}
}

func TestKeyRotator_NoRotateWhenNoReserve(t *testing.T) {
	kr := NewKeyRotator("primary-key", "", time.Hour)

	rotated := kr.RotateOnError(fmt.Errorf("HTTP 429 Too Many Requests"))
	if rotated {
		t.Fatal("should not rotate when reserve key is empty")
	}
	if kr.UsingReserve() {
		t.Fatal("should not use reserve when no reserve key")
	}
	if kr.GetKey() != "primary-key" {
		t.Fatal("key should remain primary when no reserve")
	}
}

func TestKeyRotator_ReturnToPrimaryAfterTTL(t *testing.T) {
	kr := NewKeyRotator("primary-key", "reserve-key", 50*time.Millisecond)

	// rotate to reserve
	rotated := kr.RotateOnError(fmt.Errorf("HTTP 429 Too Many Requests"))
	if !rotated {
		t.Fatal("expected rotation on 429")
	}
	if kr.GetKey() != "reserve-key" {
		t.Fatal("should be on reserve after rotation")
	}

	// before TTL expires — still on reserve
	if kr.GetKey() != "reserve-key" {
		t.Fatal("should still be on reserve before TTL")
	}

	// wait for TTL
	time.Sleep(100 * time.Millisecond)

	// after TTL — returns to primary
	if kr.GetKey() != "primary-key" {
		t.Fatalf("expected return to primary after TTL, got %s", kr.GetKey())
	}
	if kr.UsingReserve() {
		t.Fatal("should not use reserve after TTL return")
	}
}

func TestKeyRotator_Concurrency(t *testing.T) {
	kr := NewKeyRotator("primary-key", "reserve-key", time.Hour)

	var wg sync.WaitGroup
	workers := 50
	opsPerWorker := 50

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				switch (i + idx) % 3 {
				case 0:
					_ = kr.GetKey()
				case 1:
					_ = kr.UsingReserve()
				case 2:
					_ = kr.RotateOnError(fmt.Errorf("HTTP 503 error"))
				}
			}
		}(w)
	}
	wg.Wait()
}

func TestIsRateLimitOrServerError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"429 rate limit", fmt.Errorf("HTTP 429 Too Many Requests"), true},
		{"500 internal server", fmt.Errorf("HTTP 500 Internal Server Error"), true},
		{"502 bad gateway", fmt.Errorf("HTTP 502 Bad Gateway"), true},
		{"503 service unavailable", fmt.Errorf("HTTP 503 Service Unavailable"), true},
		{"504 gateway timeout", fmt.Errorf("HTTP 504 Gateway Timeout"), true},
		{"400 bad request", fmt.Errorf("HTTP 400 Bad Request"), false},
		{"401 unauthorized", fmt.Errorf("HTTP 401 Unauthorized"), false},
		{"403 forbidden", fmt.Errorf("HTTP 403 Forbidden"), false},
		{"404 not found", fmt.Errorf("HTTP 404 Not Found"), false},
		{"timeout error", fmt.Errorf("request timeout"), false},
		{"connection refused", fmt.Errorf("dial tcp: connection refused"), false},
		{"empty error", fmt.Errorf(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRateLimitOrServerError(tt.err)
			if got != tt.expected {
				t.Errorf("isRateLimitOrServerError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestKeyRotator_DoubleRotate(t *testing.T) {
	kr := NewKeyRotator("primary-key", "reserve-key", time.Hour)

	// first rotation: primary → reserve
	kr.RotateOnError(fmt.Errorf("HTTP 429 error"))
	if kr.GetKey() != "reserve-key" {
		t.Fatal("first rotation should switch to reserve")
	}
	if !kr.UsingReserve() {
		t.Fatal("should use reserve after first rotation")
	}

	// second rotation: reserve → primary
	kr.RotateOnError(fmt.Errorf("HTTP 500 error"))
	if kr.GetKey() != "primary-key" {
		t.Fatal("second rotation should switch back to primary")
	}
	if kr.UsingReserve() {
		t.Fatal("should not use reserve after second rotation")
	}
}

func ExampleKeyRotator() {
	kr := NewKeyRotator("pk", "rk", time.Hour)
	fmt.Println(kr.GetKey())

	kr.RotateOnError(fmt.Errorf("HTTP 429"))
	fmt.Println(kr.GetKey())

	// Output:
	// pk
	// rk
}
