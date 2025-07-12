package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

// LogLevel represents the logging level
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// String returns the string representation of LogLevel
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLogLevel parses log level from string
func ParseLogLevel(level string) LogLevel {
	switch level {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn":
		return WARN
	case "error":
		return ERROR
	case "fatal":
		return FATAL
	default:
		return INFO
	}
}

// Logger represents the application logger
type Logger struct {
	level  LogLevel
	format string
}

// LogEntry represents a log entry
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// NewLogger creates a new logger instance
func NewLogger(level string, format string) *Logger {
	return &Logger{
		level:  ParseLogLevel(level),
		format: format,
	}
}

// log writes a log entry
func (l *Logger) log(level LogLevel, message string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     level.String(),
		Message:   message,
		Fields:    fields,
	}

	if l.format == "json" {
		// JSON format
		jsonData, err := json.Marshal(entry)
		if err != nil {
			log.Printf("Error marshaling log entry: %v", err)
			return
		}
		fmt.Fprintln(os.Stdout, string(jsonData))
	} else {
		// Text format
		fieldsStr := ""
		if len(fields) > 0 {
			fieldsStr = fmt.Sprintf(" %v", fields)
		}
		fmt.Fprintf(os.Stdout, "[%s] %s: %s%s\n", entry.Timestamp, entry.Level, entry.Message, fieldsStr)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(message string, fields map[string]interface{}) {
	l.log(DEBUG, message, fields)
}

// Info logs an info message
func (l *Logger) Info(message string, fields map[string]interface{}) {
	l.log(INFO, message, fields)
}

// Warn logs a warning message
func (l *Logger) Warn(message string, fields map[string]interface{}) {
	l.log(WARN, message, fields)
}

// Error logs an error message
func (l *Logger) Error(message string, fields map[string]interface{}) {
	l.log(ERROR, message, fields)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(message string, fields map[string]interface{}) {
	l.log(FATAL, message, fields)
	os.Exit(1)
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...), nil)
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...), nil)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...), nil)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...), nil)
}

// Fatalf logs a formatted fatal message and exits
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.Fatal(fmt.Sprintf(format, args...), nil)
}

// WithFields creates a new logger with additional fields
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	return &Logger{
		level:  l.level,
		format: l.format,
	}
}

// Global logger instance
var globalLogger *Logger

// InitLogger initializes the global logger
func InitLogger(level string, format string) {
	globalLogger = NewLogger(level, format)
}

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	if globalLogger == nil {
		globalLogger = NewLogger("info", "json")
	}
	return globalLogger
}

// Convenience functions for global logger
func Debug(message string, fields map[string]interface{}) {
	GetLogger().Debug(message, fields)
}

func Info(message string, fields map[string]interface{}) {
	GetLogger().Info(message, fields)
}

func Warn(message string, fields map[string]interface{}) {
	GetLogger().Warn(message, fields)
}

func Error(message string, fields map[string]interface{}) {
	GetLogger().Error(message, fields)
}

func Fatal(message string, fields map[string]interface{}) {
	GetLogger().Fatal(message, fields)
}

func Debugf(format string, args ...interface{}) {
	GetLogger().Debugf(format, args...)
}

func Infof(format string, args ...interface{}) {
	GetLogger().Infof(format, args...)
}

func Warnf(format string, args ...interface{}) {
	GetLogger().Warnf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	GetLogger().Errorf(format, args...)
}

func Fatalf(format string, args ...interface{}) {
	GetLogger().Fatalf(format, args...)
}
