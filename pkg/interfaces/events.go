package interfaces

// EventType defines types of events in the system
type EventType string

const (
	// File events
	EventFileOpened   EventType = "file.opened"
	EventFileSaved    EventType = "file.saved"
	EventFileClosed   EventType = "file.closed"
	EventFileModified EventType = "file.modified"
	EventFileCreated  EventType = "file.created"
	EventFileDeleted  EventType = "file.deleted"
	EventFileRenamed  EventType = "file.renamed"
	
	// Editor events
	EventCursorMoved     EventType = "editor.cursor.moved"
	EventTextInserted    EventType = "editor.text.inserted"
	EventTextDeleted     EventType = "editor.text.deleted"
	EventSelectionChanged EventType = "editor.selection.changed"
	
	// Terminal events
	EventTerminalOutput EventType = "terminal.output"
	EventTerminalExit   EventType = "terminal.exit"
	EventTerminalResize EventType = "terminal.resize"
	
	// UI events
	EventFocusChanged  EventType = "ui.focus.changed"
	EventLayoutChanged EventType = "ui.layout.changed"
	EventModalOpened   EventType = "ui.modal.opened"
	EventModalClosed   EventType = "ui.modal.closed"
	
	// System events
	EventConfigChanged EventType = "system.config.changed"
	EventThemeChanged  EventType = "system.theme.changed"
)

// Event represents an event in the system
type Event interface {
	Type() EventType
	Source() interface{}
	Data() interface{}
	Timestamp() int64
}

// EventBus manages event publishing and subscription
type EventBus interface {
	// Publishing
	Publish(event Event)
	PublishAsync(event Event)
	
	// Subscription
	Subscribe(eventType EventType, handler EventHandler) SubscriptionID
	SubscribeOnce(eventType EventType, handler EventHandler) SubscriptionID
	Unsubscribe(id SubscriptionID)
	UnsubscribeAll(eventType EventType)
	
	// Filtering
	SubscribeWithFilter(eventType EventType, filter EventFilter, handler EventHandler) SubscriptionID
	
	// Management
	Clear()
	GetSubscriberCount(eventType EventType) int
}

// EventHandler handles events
type EventHandler func(event Event)

// EventFilter filters events
type EventFilter func(event Event) bool

// SubscriptionID identifies a subscription
type SubscriptionID string