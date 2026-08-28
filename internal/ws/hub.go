// Package ws implements a WebSocket hub with per-tenant rooms, broadcasting
// live agent activity logs as JSON.
package ws

import (
	"encoding/json"
	"sync"
	"time"
)

// Message is a single broadcast envelope.
type Message struct {
	Type    string    `json:"type"` // agent_message | run_update | task_update
	Tenant  string    `json:"tenant,omitempty"`
	Payload any       `json:"payload"`
	Time    time.Time `json:"time"`
}

// client is a single connected WebSocket subscriber.
type client struct {
	tenant string
	send   chan []byte
}

// Hub routes messages to per-tenant rooms. It is safe for concurrent use.
type Hub struct {
	mu       sync.RWMutex
	clients  map[*client]struct{}
	byTenant map[string]map[*client]struct{}
	now      func() time.Time
}

// NewHub returns an empty Hub.
func NewHub() *Hub {
	return &Hub{
		clients:  make(map[*client]struct{}),
		byTenant: make(map[string]map[*client]struct{}),
		now:      time.Now,
	}
}

// Register adds a client to the hub and returns its outbound channel plus a
// deregistration function.
func (h *Hub) Register(tenant string) (send <-chan []byte, deregister func()) {
	c := &client{tenant: tenant, send: make(chan []byte, 64)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	if h.byTenant[tenant] == nil {
		h.byTenant[tenant] = make(map[*client]struct{})
	}
	h.byTenant[tenant][c] = struct{}{}
	h.mu.Unlock()

	deregister = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.clients[c]; !ok {
			return
		}
		delete(h.clients, c)
		delete(h.byTenant[tenant], c)
		if len(h.byTenant[tenant]) == 0 {
			delete(h.byTenant, tenant)
		}
		close(c.send)
	}
	return c.send, deregister
}

// Broadcast sends a message to all clients in the given tenant. If tenant is
// empty the message goes to every client.
func (h *Hub) Broadcast(tenant, msgType string, payload any) {
	msg := Message{Type: msgType, Tenant: tenant, Payload: payload, Time: h.now()}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	deliver := func(c *client) {
		select {
		case c.send <- data:
		default:
			// Drop on a full buffer rather than blocking broadcast.
		}
	}
	if tenant == "" {
		for c := range h.clients {
			deliver(c)
		}
		return
	}
	for c := range h.byTenant[tenant] {
		deliver(c)
	}
}

// ClientCount returns the total number of registered clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
