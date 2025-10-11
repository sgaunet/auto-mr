package logger_test

import (
	"testing"

	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/stretchr/testify/assert"
)

func TestNoLogger(t *testing.T) {
	log := logger.NoLogger()

	assert.NotNil(t, log, "NoLogger should not return nil")

	// Since NoLogger should not log anything, we can call its methods and ensure no panic occurs
	assert.NotPanics(t, func() {
		log.Debug("This is a debug message")
		log.Info("This is an info message")
		log.Warn("This is a warning message")
		log.Error("This is an error message")
	}, "NoLogger methods should not panic")
}
func TestNewLogger(t *testing.T) {
	tests := []struct {
		logLevel string
	}{
		{"debug"},
		{"info"},
		{"warn"},
		{"error"},
		{""}, // Default case
	}

	for _, tt := range tests {
		t.Run(tt.logLevel, func(t *testing.T) {
			log := logger.NewLogger(tt.logLevel)
			assert.NotNil(t, log, "NewLogger should not return nil")

			// Since we cannot directly check the log level of the logger, we will ensure no panic occurs
			assert.NotPanics(t, func() {
				log.Debug("This is a debug message")
				log.Info("This is an info message")
				log.Warn("This is a warning message")
				log.Error("This is an error message")
			}, "NewLogger methods should not panic")
		})
	}
}
