package interfaces

// ConfigInterface defines the contract for configuration management
type ConfigInterface interface {
	// Loading/Saving
	Load(path string) error
	Save(path string) error
	LoadDefaults()
	
	// Getting values
	GetString(key string) string
	GetInt(key string) int
	GetBool(key string) bool
	GetFloat(key string) float64
	GetStringSlice(key string) []string
	GetMap(key string) map[string]interface{}
	
	// Setting values
	Set(key string, value interface{})
	SetDefault(key string, value interface{})
	
	// Checking existence
	IsSet(key string) bool
	
	// Nested configs
	Sub(key string) ConfigInterface
	
	// All settings
	AllSettings() map[string]interface{}
	
	// Events
	OnChange(handler func(key string, oldValue, newValue interface{}))
	
	// Validation
	Validate() error
	RegisterValidator(key string, validator ConfigValidator)
}

// ConfigValidator validates configuration values
type ConfigValidator func(value interface{}) error

// EditorConfig contains editor-specific settings
type EditorConfig struct {
	TabSize          int
	UseSpaces        bool
	AutoIndent       bool
	WordWrap         bool
	ShowLineNumbers  bool
	ShowWhitespace   bool
	HighlightLine    bool
	Theme            string
	Font             string
	FontSize         int
}

// TerminalConfig contains terminal-specific settings
type TerminalConfig struct {
	Shell            string
	ScrollbackLines  int
	CursorStyle      string
	CursorBlink      bool
	BellStyle        string
	Font             string
	FontSize         int
}

// FileManagerConfig contains file manager settings
type FileManagerConfig struct {
	ShowHidden       bool
	ShowIcons        bool
	SortBy           string
	SortReverse      bool
	ConfirmDelete    bool
	FollowSymlinks   bool
}

// KeybindingConfig represents a keybinding configuration
type KeybindingConfig struct {
	Key      string
	Command  string
	When     string // Context condition
}