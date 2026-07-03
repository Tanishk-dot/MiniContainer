package observability

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
)

func (ll LogLevel) String() string {
	switch ll {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	case LogLevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// LogEntry is a structured log message
type LogEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	Level       string                 `json:"level"`
	Message     string                 `json:"message"`
	Fields      map[string]interface{} `json:"fields,omitempty"`
	TraceID     string                 `json:"trace_id,omitempty"`
	SpanID      string                 `json:"span_id,omitempty"`
	Component   string                 `json:"component,omitempty"`
	RequestID   string                 `json:"request_id,omitempty"`
	StackTrace  string                 `json:"stack_trace,omitempty"`
}

// Logger provides structured logging
type Logger struct {
	name      string
	level     LogLevel
	output    io.Writer
	structured bool
}

// NewLogger creates a new logger
func NewLogger(name string, level LogLevel, structured bool) *Logger {
	return &Logger{
		name:       name,
		level:      level,
		output:     os.Stdout,
		structured: structured,
	}
}

// SetOutput sets the output writer
func (l *Logger) SetOutput(w io.Writer) {
	l.output = w
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, fields map[string]interface{}) {
	if l.level <= LogLevelDebug {
		l.log(LogLevelDebug, msg, fields)
	}
}

// Info logs an info message
func (l *Logger) Info(msg string, fields map[string]interface{}) {
	if l.level <= LogLevelInfo {
		l.log(LogLevelInfo, msg, fields)
	}
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, fields map[string]interface{}) {
	if l.level <= LogLevelWarn {
		l.log(LogLevelWarn, msg, fields)
	}
}

// Error logs an error message
func (l *Logger) Error(msg string, fields map[string]interface{}) {
	if l.level <= LogLevelError {
		l.log(LogLevelError, msg, fields)
	}
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, fields map[string]interface{}) {
	l.log(LogLevelFatal, msg, fields)
	os.Exit(1)
}

// log performs the actual logging
func (l *Logger) log(level LogLevel, msg string, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}

	if l.structured {
		l.logStructured(level, msg, fields)
	} else {
		l.logText(level, msg, fields)
	}
}

// logText logs in human-readable format
func (l *Logger) logText(level LogLevel, msg string, fields map[string]interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	output := fmt.Sprintf("[%s] %s %s: %s", timestamp, level.String(), l.name, msg)

	if len(fields) > 0 {
		fieldStrs := make([]string, 0, len(fields))
		for k, v := range fields {
			fieldStrs = append(fieldStrs, fmt.Sprintf("%s=%v", k, v))
		}
		output += " " + strings.Join(fieldStrs, " ")
	}

	fmt.Fprintln(l.output, output)
}

// logStructured logs in JSON format
func (l *Logger) logStructured(level LogLevel, msg string, fields map[string]interface{}) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   msg,
		Fields:    fields,
		Component: l.name,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(l.output, "error marshaling log: %v\n", err)
		return
	}

	fmt.Fprintln(l.output, string(data))
}

// LoggerPool manages multiple loggers
type LoggerPool struct {
	loggers   map[string]*Logger
	level     LogLevel
	structured bool
}

// NewLoggerPool creates a new logger pool
func NewLoggerPool(level LogLevel, structured bool) *LoggerPool {
	return &LoggerPool{
		loggers:   make(map[string]*Logger),
		level:     level,
		structured: structured,
	}
}

// Get returns or creates a logger for the given name
func (lp *LoggerPool) Get(name string) *Logger {
	if logger, exists := lp.loggers[name]; exists {
		return logger
	}

	logger := NewLogger(name, lp.level, lp.structured)
	lp.loggers[name] = logger
	return logger
}

// SetLevel sets the log level for all loggers
func (lp *LoggerPool) SetLevel(level LogLevel) {
	lp.level = level
	for _, logger := range lp.loggers {
		logger.level = level
	}
}

// GlobalLogger is the global logger pool
var GlobalLogger = NewLoggerPool(LogLevelInfo, true)

// Shorthand functions using global logger

// Debug logs a debug message
func Debug(component string, msg string, fields map[string]interface{}) {
	GlobalLogger.Get(component).Debug(msg, fields)
}

// Info logs an info message
func Info(component string, msg string, fields map[string]interface{}) {
	GlobalLogger.Get(component).Info(msg, fields)
}

// Warn logs a warning message
func Warn(component string, msg string, fields map[string]interface{}) {
	GlobalLogger.Get(component).Warn(msg, fields)
}

// Error logs an error message
func Error(component string, msg string, fields map[string]interface{}) {
	GlobalLogger.Get(component).Error(msg, fields)
}

// Fatal logs a fatal message and exits
func Fatal(component string, msg string, fields map[string]interface{}) {
	GlobalLogger.Get(component).Fatal(msg, fields)
}

// WithFields is a helper to create field maps
func WithFields(fields ...interface{}) map[string]interface{} {
	m := make(map[string]interface{})
	for i := 0; i < len(fields)-1; i += 2 {
		m[fields[i].(string)] = fields[i+1]
	}
	return m
}
