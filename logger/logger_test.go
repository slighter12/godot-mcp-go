package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogger(t *testing.T) {
	// Test log levels with JSON format
	buf := &bytes.Buffer{}
	logger := New(slog.LevelDebug, FormatJSON, buf)

	// Test debug level
	logger.Debug("debug message", "key", "value")
	output := buf.String()
	var logEntry map[string]any
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}
	if logEntry["level"] != "DEBUG" || logEntry["msg"] != "debug message" || logEntry["key"] != "value" {
		t.Error("Debug message not logged correctly")
	}
	buf.Reset()

	// Test info level
	logger.Info("info message", "key", "value")
	output = buf.String()
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}
	if logEntry["level"] != "INFO" || logEntry["msg"] != "info message" || logEntry["key"] != "value" {
		t.Error("Info message not logged correctly")
	}
	buf.Reset()

	// Test warn level
	logger.Warn("warn message", "key", "value")
	output = buf.String()
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}
	if logEntry["level"] != "WARN" || logEntry["msg"] != "warn message" || logEntry["key"] != "value" {
		t.Error("Warn message not logged correctly")
	}
	buf.Reset()

	// Test error level
	logger.Error("error message", "key", "value")
	output = buf.String()
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}
	if logEntry["level"] != "ERROR" || logEntry["msg"] != "error message" || logEntry["key"] != "value" {
		t.Error("Error message not logged correctly")
	}
	buf.Reset()

	// Test level filtering
	logger.SetLevel(slog.LevelWarn)
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")
	output = buf.String()
	lines := strings.Split(output, "\n")
	// Subtract 1 because the last line is empty
	if len(lines)-1 != 2 {
		t.Errorf("Expected 2 messages, got %d", len(lines)-1)
	}
	if !strings.Contains(output, "warn message") || !strings.Contains(output, "error message") {
		t.Error("Messages at or above warn level should be logged")
	}
}

func TestTextFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(slog.LevelInfo, FormatText, buf)

	logger.Info("test message", "key", "value")
	output := buf.String()
	if !strings.Contains(output, "test message") || !strings.Contains(output, "key=value") {
		t.Error("Text format not logged correctly")
	}
}

func TestMultipleOutputs(t *testing.T) {
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}
	logger := New(slog.LevelInfo, FormatJSON, buf1, buf2)

	logger.Info("test message", "key", "value")

	// Check both buffers
	output1 := buf1.String()
	output2 := buf2.String()

	if output1 != output2 {
		t.Error("Multiple outputs should have the same content")
	}

	var logEntry map[string]any
	if err := json.Unmarshal([]byte(output1), &logEntry); err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}
	if logEntry["msg"] != "test message" || logEntry["key"] != "value" {
		t.Error("Message not logged correctly to multiple outputs")
	}
}

func TestLogRotation(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	// Initialize logger with both stdout and file
	if err := Init(slog.LevelInfo, FormatJSON, logPath); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer defaultLogger.Close() // Ensure files are closed

	// Log some messages
	defaultLogger.Info("test message 1", "key", "value1")

	// Rotate log file
	newLogPath := filepath.Join(tempDir, "test2.log")
	if err := defaultLogger.Rotate(newLogPath); err != nil {
		t.Fatalf("Failed to rotate log file: %v", err)
	}

	// Log more messages
	defaultLogger.Info("test message 2", "key", "value2")

	// Check old log file
	oldContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read old log file: %v", err)
	}
	if !strings.Contains(string(oldContent), "test message 1") {
		t.Error("Old log file should contain first message")
	}

	// Check new log file
	newContent, err := os.ReadFile(newLogPath)
	if err != nil {
		t.Fatalf("Failed to read new log file: %v", err)
	}
	if !strings.Contains(string(newContent), "test message 2") {
		t.Error("New log file should contain second message")
	}
}

func TestLogLevelFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"invalid", slog.LevelInfo}, // Default level
	}

	for _, test := range tests {
		level := GetLevelFromString(test.input)
		if level != test.expected {
			t.Errorf("Expected level %v for input %s, got %v", test.expected, test.input, level)
		}
	}
}

func TestConcurrentLogging(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(slog.LevelDebug, FormatJSON, buf)

	// Create multiple goroutines to log messages
	done := make(chan bool)
	for i := range 10 {
		go func(id int) {
			for j := range 100 {
				logger.Info("message", "id", id, "count", j)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to finish
	for range 10 {
		<-done
	}

	// Count the number of messages
	output := buf.String()
	lines := strings.Split(output, "\n")
	// Subtract 1 because the last line is empty
	if len(lines)-1 != 1000 {
		t.Errorf("Expected 1000 messages, got %d", len(lines)-1)
	}
}
