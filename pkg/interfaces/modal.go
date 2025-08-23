package interfaces

import "github.com/gdamore/tcell/v2"

// ModalInterface defines the contract for modal dialogs
type ModalInterface interface {
	// Content
	SetTitle(title string)
	SetMessage(message string)
	SetInputValue(value string)
	GetInputValue() string
	
	// Buttons
	AddButton(label string, handler func())
	SetButtons(buttons []ModalButton)
	
	// Type
	SetType(modalType ModalType)
	GetType() ModalType
	
	// Callbacks
	OnSubmit(handler func(value string))
	OnCancel(handler func())
	OnClose(handler func())
	
	// Lifecycle
	Show()
	Close()
	IsVisible() bool
	
	// UI integration
	HandleKey(event *tcell.EventKey) bool
	Focus()
	Blur()
}

// ModalButton represents a button in a modal dialog
type ModalButton struct {
	Label   string
	Handler func()
	Default bool
	Cancel  bool
}

// ModalType defines types of modal dialogs
type ModalType int

const (
	ModalTypeInfo ModalType = iota
	ModalTypeWarning
	ModalTypeError
	ModalTypeConfirm
	ModalTypeInput
	ModalTypeProgress
)