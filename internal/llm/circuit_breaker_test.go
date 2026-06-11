package llm

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	if cb.State() != CircuitClosed {
		t.Fatal("initial state should be Closed")
	}

	// first 2 failures should keep circuit closed
	for i := 0; i < 2; i++ {
		opened := cb.RecordFailure()
		if opened {
			t.Fatalf("circuit should stay closed on failure %d", i+1)
		}
	}
	if cb.State() != CircuitClosed {
		t.Fatal("state should be Closed after 2 failures")
	}
	if cb.FailureCount() != 2 {
		t.Fatalf("expected failureCount=2, got %d", cb.FailureCount())
	}

	// 3rd failure opens the circuit
	opened := cb.RecordFailure()
	if !opened {
		t.Fatal("circuit should open on 3rd failure")
	}
	if cb.State() != CircuitOpen {
		t.Fatal("state should be Open after maxFailures")
	}
	if cb.FailureCount() != 3 {
		t.Fatalf("expected failureCount=3, got %d", cb.FailureCount())
	}
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond)

	// open the circuit immediately
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatal("expected Open after first failure")
	}

	// Allow should return false before cooldown expires
	if cb.Allow() {
		t.Fatal("expected Allow=false before cooldown")
	}

	time.Sleep(100 * time.Millisecond)

	// Allow should return true after cooldown (probe call)
	if !cb.Allow() {
		t.Fatal("expected Allow=true after cooldown for probe call")
	}
}

func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond)

	// open the circuit
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatal("expected Open")
	}

	// wait for cooldown
	time.Sleep(100 * time.Millisecond)

	// allow probe call - this transitions Open → HalfOpen
	if !cb.Allow() {
		t.Fatal("expected Allow=true after cooldown")
	}

	// record success transitions HalfOpen → Closed
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Fatal("expected Closed after success in HalfOpen")
	}
	if cb.FailureCount() != 0 {
		t.Fatalf("expected failureCount=0 after success, got %d", cb.FailureCount())
	}
}

func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond)

	// open the circuit
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatal("expected Open")
	}

	// wait for cooldown
	time.Sleep(100 * time.Millisecond)

	// allow probe call
	if !cb.Allow() {
		t.Fatal("expected Allow=true after cooldown")
	}

	// failure in HalfOpen re-opens the circuit
	opened := cb.RecordFailure()
	if !opened {
		t.Fatal("expected circuit to re-open on failure in HalfOpen")
	}
	if cb.State() != CircuitOpen {
		t.Fatal("expected Open after failure in HalfOpen")
	}
}

func TestCircuitBreaker_Cooldown(t *testing.T) {
	cb := NewCircuitBreaker(1, 100*time.Millisecond)

	// open the circuit
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatal("expected Open")
	}

	// Allow=false immediately after opening
	if cb.Allow() {
		t.Fatal("expected Allow=false immediately after opening")
	}

	// Allow=false midway through cooldown
	time.Sleep(50 * time.Millisecond)
	if cb.Allow() {
		t.Fatal("expected Allow=false midway through cooldown")
	}

	// Allow=true after cooldown expires
	time.Sleep(60 * time.Millisecond)
	if !cb.Allow() {
		t.Fatal("expected Allow=true after cooldown")
	}
}

func TestCircuitBreaker_Concurrency(t *testing.T) {
	cb := NewCircuitBreaker(100, 10*time.Millisecond)

	var wg sync.WaitGroup
	workers := 50
	opsPerWorker := 100

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				switch i % 3 {
				case 0:
					cb.Allow()
				case 1:
					cb.RecordSuccess()
				case 2:
					cb.RecordFailure()
				}
				_ = cb.State()
				_ = cb.FailureCount()
				_ = cb.State().String()
			}
		}()
	}
	wg.Wait()
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Second)

	// accumulate failures to open the circuit
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatal("expected Open after 2 failures")
	}
	if cb.FailureCount() != 2 {
		t.Fatalf("expected failureCount=2, got %d", cb.FailureCount())
	}

	// reset clears state and counter
	cb.Reset()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected Closed after Reset, got %s", cb.State())
	}
	if cb.FailureCount() != 0 {
		t.Fatalf("expected failureCount=0 after Reset, got %d", cb.FailureCount())
	}

	// after reset, Allow returns true (Closed state)
	if !cb.Allow() {
		t.Fatal("expected Allow=true after Reset")
	}
}

func TestCircuitBreaker_String(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitClosed, "CLOSED"},
		{CircuitOpen, "OPEN"},
		{CircuitHalfOpen, "HALF_OPEN"},
		{CircuitState(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func ExampleCircuitBreaker() {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)
	fmt.Println(cb.State())

	cb.RecordFailure()
	cb.RecordFailure()
	fmt.Println(cb.State())

	cb.Reset()
	fmt.Println(cb.State())

	// Output:
	// CLOSED
	// OPEN
	// CLOSED
}
