package interfaces

// Command defines the contract for executable commands (Command Pattern)
type Command interface {
	Execute() error
	Undo() error
	Redo() error
	CanUndo() bool
	GetDescription() string
}

// CommandManager manages command execution and history
type CommandManager interface {
	Execute(cmd Command) error
	Undo() error
	Redo() error
	CanUndo() bool
	CanRedo() bool
	
	// History management
	GetHistory() []Command
	ClearHistory()
	SetHistoryLimit(limit int)
	
	// Grouping
	BeginGroup(name string)
	EndGroup()
	IsInGroup() bool
	
	// Events
	OnCommandExecuted(handler func(cmd Command))
	OnUndo(handler func(cmd Command))
	OnRedo(handler func(cmd Command))
}