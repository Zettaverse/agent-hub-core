package api

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

// Sample is a single dashboard system observation. Time is unix milliseconds,
// Memory is bytes (runtime.ReadMemStats().Sys), CPU is an overall percent.
type Sample struct {
	Time             int64   `json:"time"`
	CPU              float64 `json:"cpu"`
	Memory           uint64  `json:"memory"`
	Goroutines       int     `json:"goroutines"`
	WebSocketClients int     `json:"websocket_clients"`
}

// HistoryCollector samples system metrics on a fixed interval and keeps the
// most recent `capacity` samples in a concurrency-safe ring buffer. The CPU
// sampler is injectable so tests do not depend on gopsutil timing; the client
// count source is injected the same way for the same reason.
type HistoryCollector struct {
	mu          sync.RWMutex
	capacity    int
	samples     []Sample
	head        int
	count       int
	interval    time.Duration
	cpuSampler  func() float64
	clientCount func() int
	now         func() time.Time

	stop      chan struct{}
	done      chan struct{}
	started   atomic.Bool
	startOnce sync.Once
	stopOnce  sync.Once
}

// NewHistoryCollector returns a stopped collector. cpuSampler and clientCount
// default to the real gopsutil CPU sampler and a zero client count when nil.
func NewHistoryCollector(capacity int, interval time.Duration, cpuSampler func() float64, clientCount func() int) *HistoryCollector {
	if capacity < 1 {
		capacity = 1
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if cpuSampler == nil {
		cpuSampler = defaultCPUSampler
	}
	if clientCount == nil {
		clientCount = func() int { return 0 }
	}
	return &HistoryCollector{
		capacity:    capacity,
		samples:     make([]Sample, capacity),
		interval:    interval,
		cpuSampler:  cpuSampler,
		clientCount: clientCount,
		now:         time.Now,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
}

// defaultCPUSampler returns the overall CPU utilization percent. On error it
// returns 0 so the dashboard never fails because of a metrics hiccup.
func defaultCPUSampler() float64 {
	vals, err := cpu.Percent(0, false)
	if err != nil || len(vals) == 0 {
		return 0
	}
	return vals[0]
}

// Capture records a single sample now and returns it.
func (c *HistoryCollector) Capture() Sample {
	s := Sample{
		Time:             c.now().UnixMilli(),
		CPU:              c.cpuSampler(),
		Memory:           readMemoryBytes(),
		Goroutines:       runtime.NumGoroutine(),
		WebSocketClients: c.clientCount(),
	}
	c.mu.Lock()
	c.write(s)
	c.mu.Unlock()
	return s
}

// Latest returns the most recent sample. If nothing has been captured yet it
// returns the zero Sample (Time == 0).
func (c *HistoryCollector) Latest() Sample {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.count == 0 {
		return Sample{}
	}
	return c.samples[c.index(c.head-1)]
}

// Snapshot returns a copy of the buffered samples in chronological order
// (oldest first). It never shares the underlying buffer, so callers may mutate
// the result freely. When empty it returns a non-nil empty slice so the JSON
// serialization is [] rather than null.
func (c *HistoryCollector) Snapshot() []Sample {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Sample, 0, c.count)
	for i := 0; i < c.count; i++ {
		out = append(out, c.samples[c.index(c.head-c.count+i)])
	}
	return out
}

// Start launches the background sampler. It is idempotent and returns
// immediately; the first sample is captured in the background goroutine so
// startup is never blocked by a metrics read.
func (c *HistoryCollector) Start() {
	c.startOnce.Do(func() {
		c.started.Store(true)
		go c.run()
	})
}

// Stop terminates the background sampler. If Start was never called it returns
// immediately; otherwise it waits for the run loop to exit.
func (c *HistoryCollector) Stop() {
	c.stopOnce.Do(func() { close(c.stop) })
	if c.started.Load() {
		<-c.done
	}
}

func (c *HistoryCollector) run() {
	defer close(c.done)

	c.Capture() // seed immediately so the dashboard has data before the first tick
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.Capture()
		case <-c.stop:
			return
		}
	}
}

// write appends s to the ring buffer, overwriting the oldest entry once full.
// The caller must hold c.mu for writing.
func (c *HistoryCollector) write(s Sample) {
	c.samples[c.head] = s
	c.head = (c.head + 1) % c.capacity
	if c.count < c.capacity {
		c.count++
	}
}

// index normalizes an offset (possibly negative) into [0, capacity).
func (c *HistoryCollector) index(offset int) int {
	return (offset%c.capacity + c.capacity) % c.capacity
}

// readMemoryBytes returns the bytes of memory obtained from the runtime (Sys),
// which tracks the total bytes obtained from the OS by the Go runtime.
func readMemoryBytes() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Sys
}
