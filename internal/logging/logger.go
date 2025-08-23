package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

type Logger struct {
	level      LogLevel
	file       *os.File
	fileLogger *log.Logger
	mu         sync.Mutex
	enabled    bool
}

var (
	instance *Logger
	once     sync.Once
)

// GetLogger returns the singleton logger instance
func GetLogger() *Logger {
	once.Do(func() {
		instance = &Logger{
			level:   INFO,
			enabled: true,
		}
		instance.initLogFile()
	})
	return instance
}

func (l *Logger) initLogFile() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	
	logDir := filepath.Join(homeDir, ".emed")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return
	}
	
	// Clean up old log files (keep last 7 days)
	l.cleanOldLogs(logDir, 7)
	
	logFile := filepath.Join(logDir, fmt.Sprintf("emed_%s.log", time.Now().Format("2006-01-02")))
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	
	l.file = file
	l.fileLogger = log.New(file, "", log.Ldate|log.Ltime|log.Lmicroseconds)
}

func (l *Logger) cleanOldLogs(logDir string, keepDays int) {
	files, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	
	cutoffTime := time.Now().AddDate(0, 0, -keepDays)
	
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		
		// Check if it's a log file
		if !strings.HasPrefix(file.Name(), "emed_") || !strings.HasSuffix(file.Name(), ".log") {
			continue
		}
		
		// Parse date from filename
		dateStr := strings.TrimPrefix(file.Name(), "emed_")
		dateStr = strings.TrimSuffix(dateStr, ".log")
		
		fileDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		
		// Delete if older than cutoff
		if fileDate.Before(cutoffTime) {
			oldFile := filepath.Join(logDir, file.Name())
			os.Remove(oldFile)
		}
	}
}

func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if !l.enabled || level < l.level {
		return
	}
	
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.fileLogger == nil {
		return
	}
	
	levelStr := [...]string{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"}[level]
	msg := fmt.Sprintf("[%s] %s", levelStr, fmt.Sprintf(format, args...))
	l.fileLogger.Println(msg)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, format, args...)
	if l.file != nil {
		l.file.Sync()
		l.file.Close()
	}
	os.Exit(1)
}

func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.file != nil {
		l.file.Sync()
		l.file.Close()
		l.file = nil
		l.fileLogger = nil
	}
}

// Convenience functions for direct logging
func Debug(format string, args ...interface{}) {
	GetLogger().Debug(format, args...)
}

func Info(format string, args ...interface{}) {
	GetLogger().Info(format, args...)
}

func Warn(format string, args ...interface{}) {
	GetLogger().Warn(format, args...)
}

func Error(format string, args ...interface{}) {
	GetLogger().Error(format, args...)
}

func Fatal(format string, args ...interface{}) {
	GetLogger().Fatal(format, args...)
}