package interfaces

// StatusBarInterface defines the contract for status bar implementations
type StatusBarInterface interface {
	// Content updates
	SetFilePath(path string)
	SetPosition(line, col int)
	SetMode(mode string)
	SetMessage(message string, duration int) // duration in seconds, 0 = permanent
	SetGitBranch(branch string)
	SetModified(modified bool)
	
	// Indicators
	ShowProgress(label string, current, total int)
	HideProgress()
	SetIndicator(key, value string)
	RemoveIndicator(key string)
	
	// Lifecycle
	Update()
	Clear()
	
	// Events
	OnClick(handler func(x, y int))
}