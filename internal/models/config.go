package models

import "time"

// Config represents the application configuration
type Config struct {
	MaxConcurrency   int64
	RequestsPerSec   float64
	RequestTimeout   time.Duration
	ShutdownTimeout  time.Duration
	EmailsFilePath   string
	TokensFilePath   string
	AccountsFilePath string
	MinTokens        int
	MaxTokens        int
	SleepDuration    time.Duration
}
