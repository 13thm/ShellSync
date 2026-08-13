package eventbus

import (
	"sync"
	"testing"
	"time"
)

func TestPublishSubscribe(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe()
	defer cancel()

	b.Publish(Event{Type: "task.created", Entity: "task", Action: "created", Payload: "x"})

	select {
	case e := <-ch:
		if e.Type != "task.created" {
			t.Fatalf("type = %q", e.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("event not received")
	}
}

func TestCancelStopsDelivery(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe()
	cancel()

	// publishing after cancel must not panic
	b.Publish(Event{Type: "task.updated"})

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed")
	}
}

func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	b := New()
	// subscriber that never drains (buffer 64)
	_, cancel := b.Subscribe()
	defer cancel()

	done := make(chan struct{})
	go func() {
		// publish more than the buffer
		for i := 0; i < 200; i++ {
			b.Publish(Event{Type: "task.updated"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on slow subscriber")
	}

	// silence unused var warning in race-free builds
	var _ = sync.Mutex{}
}
