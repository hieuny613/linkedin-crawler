package config

import (
	"time"

	"linkedin-crawler/internal/models"
)

// DefaultConfig returns the default configuration for the crawler
func DefaultConfig() models.Config {
	return models.Config{
		MaxConcurrency:   100,
		RequestsPerSec:   30.0,
		RequestTimeout:   15 * time.Second,
		ShutdownTimeout:  10 * time.Second,
		EmailsFilePath:   "emails.txt",
		TokensFilePath:   "tokens.txt",
		AccountsFilePath: "accounts.txt",
		MinTokens:        10,
		MaxTokens:        10,
		SleepDuration:    30 * time.Second,
	}
}
