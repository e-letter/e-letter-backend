package handler

import (
	"io"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

type EventBus struct {
	mu          sync.RWMutex
	subscribers map[int][]chan string
}

func NewEventBus() *EventBus {
	return &EventBus{subscribers: make(map[int][]chan string)}
}

func (eb *EventBus) Subscribe(userID int) chan string {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	ch := make(chan string, 10)
	eb.subscribers[userID] = append(eb.subscribers[userID], ch)
	return ch
}

func (eb *EventBus) Unsubscribe(userID int, ch chan string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	subs := eb.subscribers[userID]
	for i, s := range subs {
		if s == ch {
			eb.subscribers[userID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}

func (eb *EventBus) Publish(userID int, event string) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	for _, ch := range eb.subscribers[userID] {
		select {
		case ch <- event:
		default:
		}
	}
}

type SSEHandler struct {
	bus *EventBus
}

func NewSSEHandler(bus *EventBus) *SSEHandler {
	return &SSEHandler{bus: bus}
}

func (h *SSEHandler) Stream(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	ch := h.bus.Subscribe(userID)
	defer h.bus.Unsubscribe(userID, ch)

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-ch:
			if !ok {
				return false
			}
			c.SSEvent("message", msg)
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}
