// Package logger provides logging utilities for auto-mr using the bullets library.
//
// It wraps [bullets.Logger] with convenience constructors for creating loggers
// at various levels and a silent logger for use in tests or when no output is desired.
//
// Usage:
//
//	log := logger.NewLogger("debug")
//	log.Debug("Starting operation")
//
//	silentLog := logger.NoLogger() // Suppresses all output
package logger

import (
	"os"

	"github.com/sgaunet/bullets"
)

// Logger is the interface for logging in auto-mr.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// NewLogger creates a new logger that writes to stdout at the specified level.
//
// Parameters:
//   - logLevel: one of "debug", "info", "warn", "error" (defaults to "info" for unknown values)
func NewLogger(logLevel string) *bullets.Logger {
	var level bullets.Level
	switch logLevel {
	case "debug":
		level = bullets.DebugLevel
	case "info":
		level = bullets.InfoLevel
	case "warn":
		level = bullets.WarnLevel
	case "error":
		level = bullets.ErrorLevel
	default:
		level = bullets.InfoLevel
	}
	logger := bullets.New(os.Stdout)
	logger.SetLevel(level)
	return logger
}

// NoLogger creates a logger that suppresses all output by setting the level to Fatal.
// Useful for tests and silent operation.
func NoLogger() *bullets.Logger {
	logger := bullets.New(os.Stdout)
	logger.SetLevel(bullets.FatalLevel)
	return logger
}
