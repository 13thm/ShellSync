// Package eventbus is the in-process domain-event bus.
//
// Services publish entity-change events; the WebSocket layer subscribes and
// fans them out to connected clients so all clients stay in sync.
package eventbus

// Event is a domain change broadcast to subscribers.
type Event struct {
	// Type is the dotted event name, e.g. "task.created".
	Type string
	// Entity is "task" | "terminal" | "todo".
	Entity string
	// Action is "created" | "updated" | "deleted".
	Action string
	// Payload is the (DTO-ready) entity data, or a map for deletes.
	Payload any
}
