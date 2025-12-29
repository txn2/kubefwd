package events

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

// Handler is a function that handles events
type Handler func(Event)

// UnsubscribeFunc is returned by Subscribe and can be called to remove the handler
type UnsubscribeFunc func()

// handlerEntry wraps a handler with a unique ID for unsubscription
type handlerEntry struct {
	id      uint64
	handler Handler
}

// Bus is a thread-safe event bus for TUI updates
type Bus struct {
	mu        sync.RWMutex
	handlers  map[EventType][]handlerEntry
	allHandle []handlerEntry // handlers for all event types
	nextID    uint64         // monotonically increasing handler ID
	eventChan chan Event
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

// NewBus creates a new event bus with the specified buffer size
func NewBus(bufferSize int) *Bus {
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	return &Bus{
		handlers:  make(map[EventType][]handlerEntry),
		allHandle: make([]handlerEntry, 0),
		eventChan: make(chan Event, bufferSize),
		stopChan:  make(chan struct{}),
	}
}

// Subscribe adds a handler for a specific event type and returns an unsubscribe function.
// Call the returned function to remove the handler and prevent memory leaks.
func (b *Bus) Subscribe(eventType EventType, handler Handler) UnsubscribeFunc {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++

	b.handlers[eventType] = append(b.handlers[eventType], handlerEntry{id: id, handler: handler})

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		handlers := b.handlers[eventType]
		for i, entry := range handlers {
			if entry.id == id {
				// Remove by swapping with last element and truncating
				handlers[i] = handlers[len(handlers)-1]
				b.handlers[eventType] = handlers[:len(handlers)-1]
				return
			}
		}
	}
}

// SubscribeAll adds a handler for all event types and returns an unsubscribe function.
// Call the returned function to remove the handler and prevent memory leaks.
func (b *Bus) SubscribeAll(handler Handler) UnsubscribeFunc {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++

	b.allHandle = append(b.allHandle, handlerEntry{id: id, handler: handler})

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for i, entry := range b.allHandle {
			if entry.id == id {
				// Remove by swapping with last element and truncating
				b.allHandle[i] = b.allHandle[len(b.allHandle)-1]
				b.allHandle = b.allHandle[:len(b.allHandle)-1]
				return
			}
		}
	}
}

// Publish sends an event to all subscribed handlers
// This is non-blocking - if the buffer is full, the event is dropped
func (b *Bus) Publish(event Event) {
	select {
	case b.eventChan <- event:
	default:
		// Buffer full, drop event to prevent blocking
	}
}

// Start begins processing events in a background goroutine
func (b *Bus) Start() {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for {
			select {
			case event := <-b.eventChan:
				b.dispatch(event)
			case <-b.stopChan:
				// Drain remaining events
				for {
					select {
					case event := <-b.eventChan:
						b.dispatch(event)
					default:
						return
					}
				}
			}
		}
	}()
}

// dispatch sends an event to all appropriate handlers
func (b *Bus) dispatch(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Call type-specific handlers
	if handlers, ok := b.handlers[event.Type]; ok {
		for _, entry := range handlers {
			b.safeCall(entry.handler, event)
		}
	}

	// Call handlers subscribed to all events
	for _, entry := range b.allHandle {
		b.safeCall(entry.handler, event)
	}
}

// safeCall invokes a handler with panic recovery to prevent one bad handler
// from crashing the entire event bus.
func (b *Bus) safeCall(handler Handler, event Event) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Event handler panic for %s: %v", event.Type, r)
		}
	}()
	handler(event)
}

// Stop stops the event bus and waits for pending events to be processed
func (b *Bus) Stop() {
	close(b.stopChan)
	b.wg.Wait()
}

// EventChan returns the event channel for direct access if needed
func (b *Bus) EventChan() <-chan Event {
	return b.eventChan
}
