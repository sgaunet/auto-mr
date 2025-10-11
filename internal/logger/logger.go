// Package logger provides logging utilities for auto-mr.
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

// NewLogger creates a new logger
// logLevel is the level of logging
// Possible values of logLevel are: "debug", "info", "warn", "error"
// Default value is "info".
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

// NoLogger creates a logger that does not log anything.
func NoLogger() *bullets.Logger {
	logger := bullets.New(os.Stdout)
	logger.SetLevel(bullets.FatalLevel)
	return logger
}
