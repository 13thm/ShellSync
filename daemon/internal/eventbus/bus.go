package eventbus

import "sync"

// Bus is an in-process pub/sub event bus.
type Bus struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

// New creates an empty bus.
func New() *Bus {
	return &Bus{subs: map[chan Event]struct{}{}}
}

// Publish broadcasts an event to all subscribers. A slow subscriber's event is
// dropped rather than blocking the publisher (events are also delivered live
// via REST/WS, so a dropped sync event is non-fatal).
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Subscribe returns a channel that receives events plus a cancel function that
// unsubscribes and closes the channel.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}
