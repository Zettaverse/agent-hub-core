package api

import (
	"testing"
	"time"
)

func TestHistoryCollectorRingBufferEviction(t *testing.T) {
	var counter float64
	now := time.Unix(1700000000, 0)
	c := NewHistoryCollector(3, time.Second, func() float64 {
		counter++
		return counter
	}, func() int { return 7 })
	c.now = func() time.Time { return now }

	// Capture five samples into a three-slot buffer.
	for i := 0; i < 5; i++ {
		now = now.Add(time.Second)
		c.Capture()
	}

	got := c.Snapshot()
	if len(got) != 3 {
		t.Fatalf("snapshot length = %d, want 3", len(got))
	}
	// The oldest two (CPU 1 and 2) must have been evicted; 3,4,5 remain in
	// chronological order.
	wantCPUs := []float64{3, 4, 5}
	for i, s := range got {
		if s.CPU != wantCPUs[i] {
			t.Fatalf("snapshot[%d].cpu = %f, want %f", i, s.CPU, wantCPUs[i])
		}
		if s.WebSocketClients != 7 {
			t.Fatalf("snapshot[%d].websocket_clients = %d, want 7", i, s.WebSocketClients)
		}
	}
}

func TestHistoryCollectorSnapshotIsCopy(t *testing.T) {
	c := NewHistoryCollector(2, time.Second, func() float64 { return 1.5 }, nil)
	c.now = func() time.Time { return time.Unix(1700000000, 0) }

	c.Capture()

	snap := c.Snapshot()
	snap[0].CPU = 999

	again := c.Snapshot()
	if again[0].CPU != 1.5 {
		t.Fatalf("snapshot shares underlying buffer: cpu = %f, want 1.5", again[0].CPU)
	}
}

func TestHistoryCollectorLatestEmpty(t *testing.T) {
	c := NewHistoryCollector(2, time.Second, func() float64 { return 1.5 }, nil)

	latest := c.Latest()
	if latest.Time != 0 {
		t.Fatalf("empty collector Latest() time = %d, want 0", latest.Time)
	}
}

func TestHistoryCollectorDefaults(t *testing.T) {
	// Capacity/interval defaults must not panic or misbehave on zero config.
	c := NewHistoryCollector(0, 0, func() float64 { return 42 }, nil)
	c.now = func() time.Time { return time.Unix(1700000000, 0) }

	s := c.Capture()
	if s.CPU != 42 {
		t.Fatalf("cpu sampler returned %f, want 42", s.CPU)
	}
	if s.WebSocketClients != 0 {
		t.Fatalf("nil clientCount default returned %d, want 0", s.WebSocketClients)
	}
	if c.capacity != 1 {
		t.Fatalf("capacity default = %d, want 1", c.capacity)
	}
	if c.interval != 5*time.Second {
		t.Fatalf("interval default = %s, want 5s", c.interval)
	}
}
