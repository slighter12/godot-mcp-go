package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Format represents the log format
type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

// Logger represents a logger instance
type Logger struct {
	*slog.Logger
	mu      sync.Mutex
	writers []io.Writer
	level   slog.Level
	format  Format
}

// New creates a new logger
func New(level slog.Level, format Format, writers ...io.Writer) *Logger {
	multiWriter := io.MultiWriter(writers...)
	var handler slog.Handler
	switch format {
	case FormatJSON:
		handler = slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{
			Level: level,
		})
	case FormatText:
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
			Level: level,
		})
	default:
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
			Level: level,
		})
	}
	return &Logger{
		Logger:  slog.New(handler),
		writers: writers,
		level:   level,
		format:  format,
	}
}

// SetLevel sets the logging level
func (l *Logger) SetLevel(level slog.Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
	multiWriter := io.MultiWriter(l.writers...)
	var handler slog.Handler
	switch l.format {
	case FormatJSON:
		handler = slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{
			Level: level,
		})
	case FormatText:
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
			Level: level,
		})
	default:
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
			Level: level,
		})
	}
	l.Logger = slog.New(handler)
}

// AddOutput adds a new output destination
func (l *Logger) AddOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writers = append(l.writers, w)
	multiWriter := io.MultiWriter(l.writers...)
	var handler slog.Handler
	switch l.format {
	case FormatJSON:
		handler = slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{
			Level: l.level,
		})
	case FormatText:
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
			Level: l.level,
		})
	default:
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
			Level: l.level,
		})
	}
	l.Logger = slog.New(handler)
}

// SetFormat changes the log format
func (l *Logger) SetFormat(format Format) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.format = format
	multiWriter := io.MultiWriter(l.writers...)
	var handler slog.Handler
	switch format {
	case FormatJSON:
		handler = slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{
			Level: l.level,
		})
	case FormatText:
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
			Level: l.level,
		})
	default:
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
			Level: l.level,
		})
	}
	l.Logger = slog.New(handler)
}

// Rotate rotates the log file
func (l *Logger) Rotate(path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Find and close the current file if it exists
	var newWriters []io.Writer
	for _, writer := range l.writers {
		if file, ok := writer.(*os.File); ok {
			// Don't close stdout/stderr
			if file != os.Stdout && file != os.Stderr {
				file.Close()
			} else {
				newWriters = append(newWriters, writer)
			}
		} else {
			// Keep non-file writers
			newWriters = append(newWriters, writer)
		}
	}

	// Create a new file
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	// Add the new file to writers
	newWriters = append(newWriters, file)
	l.writers = newWriters

	// Create new handler with all writers
	multiWriter := io.MultiWriter(l.writers...)
	var handler slog.Handler
	switch l.format {
	case FormatJSON:
		handler = slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{
			Level: l.level,
		})
	case FormatText:
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
			Level: l.level,
		})
	default:
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
			Level: l.level,
		})
	}
	l.Logger = slog.New(handler)
	return nil
}

// Close closes all file writers
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, writer := range l.writers {
		if file, ok := writer.(*os.File); ok {
			// Don't close stdout/stderr
			if file != os.Stdout && file != os.Stderr {
				if err := file.Close(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Init initializes the default logger
func Init(level slog.Level, format Format, paths ...string) error {
	var writers []io.Writer
	writers = append(writers, os.Stdout) // Always include stdout

	// Add file writers if paths are provided
	for _, path := range paths {
		if path != "" {
			// Create log directory if it doesn't exist
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}

			// Open log file
			file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return err
			}
			writers = append(writers, file)
		}
	}

	// Create logger
	defaultLogger = New(level, format, writers...)
	return nil
}

// GetLevelFromString returns the log level from a string
func GetLevelFromString(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// defaultLogger is the default logger instance
var defaultLogger *Logger

// Helper functions for common logging patterns
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

func DebugContext(ctx context.Context, msg string, args ...any) {
	defaultLogger.DebugContext(ctx, msg, args...)
}

func InfoContext(ctx context.Context, msg string, args ...any) {
	defaultLogger.InfoContext(ctx, msg, args...)
}

func WarnContext(ctx context.Context, msg string, args ...any) {
	defaultLogger.WarnContext(ctx, msg, args...)
}

func ErrorContext(ctx context.Context, msg string, args ...any) {
	defaultLogger.ErrorContext(ctx, msg, args...)
}

// Level returns the current log level
func (l *Logger) Level() slog.Level {
	return l.level
}
