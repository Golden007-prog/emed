package interfaces

// LogLevel defines logging levels
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
)

// Logger defines the contract for logging
type Logger interface {
	// Basic logging
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
	
	// Formatted logging
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	
	// With context
	With(fields ...Field) Logger
	WithError(err error) Logger
	
	// Configuration
	SetLevel(level LogLevel)
	GetLevel() LogLevel
	SetOutput(output LogOutput)
	
	// Child loggers
	Named(name string) Logger
}

// Field represents a structured logging field
type Field struct {
	Key   string
	Value interface{}
}

// LogOutput defines where logs are written
type LogOutput interface {
	Write(entry LogEntry) error
	Close() error
}

// LogEntry represents a log entry
type LogEntry struct {
	Level     LogLevel
	Message   string
	Fields    []Field
	Timestamp int64
	Logger    string
}