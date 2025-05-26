package utils

import (
	"fmt"
	"os"
	"time"
)

// PrintErr prints error message to stderr
func PrintErr(message string) {
	if message != "" {
		fmt.Fprintln(os.Stderr, message)
	}
}

// FormatDuration formats duration in a human-readable way
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", m, s)
	} else {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh%dm", h, m)
	}
}
