package emitter

import (
	"encoding/json"
	"log"

	"no.cap/goddb/pkg/events"
)

// EventEmitter defines the interface for sending visualization events.
type EventEmitter interface {
	Emit(event events.Event) error
	Close() error
}

// LogEmitter is a simple implementation that logs events to stdout.
type LogEmitter struct{}

func NewLogEmitter() *LogEmitter {
	log.Println("Initializing LogEmitter for visualization events.")
	return &LogEmitter{}
}

func (e *LogEmitter) Emit(event events.Event) error {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshalling event for logging: %v", err)
		return err // Don't log the raw struct if marshalling fails
	}
	// Log with a prefix to make filtering easier
	log.Printf("[VIS_EVENT] %s", string(eventJSON))
	return nil
}

func (e *LogEmitter) Close() error {
	log.Println("Closing LogEmitter.")
	// No resources to release for LogEmitter
	return nil
}

// --- Null Emitter (for disabling events easily) ---

// NullEmitter is an implementation that does nothing.
type NullEmitter struct{}

func NewNullEmitter() *NullEmitter {
	return &NullEmitter{}
}

func (e *NullEmitter) Emit(event events.Event) error {
	// Do nothing
	return nil
}

func (e *NullEmitter) Close() error {
	// Do nothing
	return nil
}

// TODO: Implement WebSocket/HTTP Emitter
// type HttpEventEmitter struct {
// 	 TargetURL string
// 	 HttpClient *http.Client
// }
// 
// func NewHttpEventEmitter(targetURL string) *HttpEventEmitter { ... }
// func (e *HttpEventEmitter) Emit(event events.Event) error { ... }
// func (e *HttpEventEmitter) Close() error { ... } 