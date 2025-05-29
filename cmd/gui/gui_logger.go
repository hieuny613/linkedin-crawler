package main

import (
	"fmt"
)

// GUILogger interface for sending logs to GUI components
type GUILogger interface {
	LogInfo(message string)
	LogWarning(message string)
	LogError(message string)
	LogSuccess(message string)
	UpdateProgress(processed, total int, message string)
}

// =============================================================================
// EmailsTab implements GUILogger interface
// =============================================================================

func (et *EmailsTab) LogInfo(message string) {
	et.gui.updateUI <- func() {
		et.addLog(fmt.Sprintf("ℹ️ %s", message))
	}
}

func (et *EmailsTab) LogWarning(message string) {
	et.gui.updateUI <- func() {
		et.addLog(fmt.Sprintf("⚠️ %s", message))
	}
}

func (et *EmailsTab) LogError(message string) {
	et.gui.updateUI <- func() {
		et.addLog(fmt.Sprintf("❌ %s", message))
	}
}

func (et *EmailsTab) LogSuccess(message string) {
	et.gui.updateUI <- func() {
		et.addLog(fmt.Sprintf("✅ %s", message))
	}
}

func (et *EmailsTab) UpdateProgress(processed, total int, message string) {
	et.gui.updateUI <- func() {
		et.addLog(fmt.Sprintf("📊 %s", message))

		// Update progress in status bar instead of control tab
		if total > 0 {
			progress := float64(processed) / float64(total)
			progressMsg := fmt.Sprintf("Progress: %d/%d (%.1f%%)", processed, total, progress*100)
			et.gui.updateStatus(progressMsg)
		}
	}
}

// =============================================================================
// AccountsTab implements GUILogger interface (for token extraction)
// =============================================================================

func (at *AccountsTab) LogInfo(message string) {
	at.gui.updateUI <- func() {
		at.addLog(fmt.Sprintf("ℹ️ %s", message))
	}
}

func (at *AccountsTab) LogWarning(message string) {
	at.gui.updateUI <- func() {
		at.addLog(fmt.Sprintf("⚠️ %s", message))
	}
}

func (at *AccountsTab) LogError(message string) {
	at.gui.updateUI <- func() {
		at.addLog(fmt.Sprintf("❌ %s", message))
	}
}

func (at *AccountsTab) LogSuccess(message string) {
	at.gui.updateUI <- func() {
		at.addLog(fmt.Sprintf("✅ %s", message))
	}
}

func (at *AccountsTab) UpdateProgress(processed, total int, message string) {
	at.gui.updateUI <- func() {
		at.addLog(fmt.Sprintf("📊 %s", message))

		// Update token extraction progress if needed
		if total > 0 {
			progress := float64(processed) / float64(total)
			progressMsg := fmt.Sprintf("Token extraction: %.1f%% (%d/%d)", progress*100, processed, total)
			at.addLog(progressMsg)
			// Update status bar with token extraction progress
			at.gui.updateStatus(fmt.Sprintf("Extracting tokens: %.1f%%", progress*100))
		}
	}
}

// =============================================================================
// Helper functions for GUI Logger integration
// =============================================================================

// GetPrimaryGUILogger returns the primary GUI logger (EmailsTab)
func (gui *CrawlerGUI) GetPrimaryGUILogger() GUILogger {
	if gui.emailsTab != nil {
		return gui.emailsTab
	}
	return nil
}

// GetAccountsGUILogger returns the accounts tab as GUI logger
func (gui *CrawlerGUI) GetAccountsGUILogger() GUILogger {
	if gui.accountsTab != nil {
		return gui.accountsTab
	}
	return nil
}

// BroadcastLog sends log message to all GUI components that implement GUILogger
func (gui *CrawlerGUI) BroadcastLog(logType string, message string) {
	loggers := []GUILogger{}

	if gui.emailsTab != nil {
		loggers = append(loggers, gui.emailsTab)
	}
	if gui.accountsTab != nil {
		loggers = append(loggers, gui.accountsTab)
	}

	for _, logger := range loggers {
		switch logType {
		case "info":
			logger.LogInfo(message)
		case "warning":
			logger.LogWarning(message)
		case "error":
			logger.LogError(message)
		case "success":
			logger.LogSuccess(message)
		}
	}
}

// LogToGUI is a convenience function to log to the primary GUI logger
func (gui *CrawlerGUI) LogToGUI(logType string, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logger := gui.GetPrimaryGUILogger()

	if logger != nil {
		switch logType {
		case "info":
			logger.LogInfo(message)
		case "warning":
			logger.LogWarning(message)
		case "error":
			logger.LogError(message)
		case "success":
			logger.LogSuccess(message)
		}
	}
}

// UpdateGUIProgress updates progress in all relevant GUI components
func (gui *CrawlerGUI) UpdateGUIProgress(processed, total int, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)

	// Update primary logger (emails tab)
	if gui.emailsTab != nil {
		gui.emailsTab.UpdateProgress(processed, total, message)
	}
}

// =============================================================================
// Utility functions for formatted logging
// =============================================================================

// LogInfo logs info message to GUI
func (gui *CrawlerGUI) LogInfo(format string, args ...interface{}) {
	gui.LogToGUI("info", format, args...)
}

// LogWarning logs warning message to GUI
func (gui *CrawlerGUI) LogWarning(format string, args ...interface{}) {
	gui.LogToGUI("warning", format, args...)
}

// LogError logs error message to GUI
func (gui *CrawlerGUI) LogError(format string, args ...interface{}) {
	gui.LogToGUI("error", format, args...)
}

// LogSuccess logs success message to GUI
func (gui *CrawlerGUI) LogSuccess(format string, args ...interface{}) {
	gui.LogToGUI("success", format, args...)
}

// =============================================================================
// Integration with orchestrator components
// =============================================================================

// SetupGUILoggerForOrchestrator sets up GUI logger for orchestrator components
func (gui *CrawlerGUI) SetupGUILoggerForOrchestrator(autoCrawler interface{}) {
	// This function can be called when autoCrawler is created
	// to set up GUI logging for all orchestrator components

	// Example usage in emails_tab.go:
	// autoCrawler, err := orchestrator.New(cfg)
	// gui.SetupGUILoggerForOrchestrator(autoCrawler)

	gui.LogInfo("🔧 Setting up GUI logger for orchestrator components")

	// Additional setup can be added here for other orchestrator components
	// that need GUI logging integration
}

// =============================================================================
// Constants for log formatting
// =============================================================================

const (
	LogTypeInfo    = "info"
	LogTypeWarning = "warning"
	LogTypeError   = "error"
	LogTypeSuccess = "success"
)

// Icon constants for different log types
const (
	IconInfo     = "ℹ️"
	IconWarning  = "⚠️"
	IconError    = "❌"
	IconSuccess  = "✅"
	IconProgress = "📊"
	IconToken    = "🔑"
	IconEmail    = "📧"
	IconAccount  = "👥"
	IconCrawler  = "🕷️"
	IconFile     = "📁"
	IconSave     = "💾"
	IconRefresh  = "🔄"
	IconComplete = "🎉"
)
