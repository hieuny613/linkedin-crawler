package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// NewControlTab creates a new control tab
func NewControlTab(gui *CrawlerGUI) *ControlTab {
	tab := &ControlTab{
		gui: gui,
	}

	// Initialize buttons
	tab.startBtn = widget.NewButtonWithIcon("Start Crawler", theme.MediaPlayIcon(), tab.StartCrawler)
	tab.stopBtn = widget.NewButtonWithIcon("Stop Crawler", theme.MediaStopIcon(), tab.StopCrawler)
	tab.pauseBtn = widget.NewButtonWithIcon("Pause", theme.MediaPauseIcon(), tab.PauseCrawler)
	tab.resumeBtn = widget.NewButtonWithIcon("Resume", theme.MediaPlayIcon(), tab.ResumeCrawler)

	// Style buttons
	tab.startBtn.Importance = widget.HighImportance
	tab.stopBtn.Importance = widget.DangerImportance
	tab.pauseBtn.Importance = widget.MediumImportance
	tab.resumeBtn.Importance = widget.HighImportance

	// Initialize progress
	tab.progressBar = widget.NewProgressBar()
	tab.progressLabel = widget.NewLabel("Ready to start")

	// Initialize stats labels
	tab.processedLabel = widget.NewLabel("Processed: 0")
	tab.successLabel = widget.NewLabel("Success: 0")
	tab.failedLabel = widget.NewLabel("Failed: 0")
	tab.tokensLabel = widget.NewLabel("Tokens: 0")
	tab.rateLabel = widget.NewLabel("Rate: 0.0/s")

	// Initialize status labels
	tab.statusLabel = widget.NewLabel("Status: Ready")
	tab.timeLabel = widget.NewLabel("Time: 00:00:00")

	// Set initial button states
	tab.updateButtonStates(false)

	return tab
}

// CreateContent creates the control tab content
func (ct *ControlTab) CreateContent() fyne.CanvasObject {
	// Control buttons section
	controlButtons := container.NewHBox(
		ct.startBtn,
		ct.stopBtn,
		ct.pauseBtn,
		ct.resumeBtn,
	)

	controlCard := widget.NewCard("Crawler Control",
		"Start, stop, pause, and resume the crawling process",
		container.NewVBox(controlButtons))

	// Progress section
	progressContent := container.NewVBox(
		ct.progressLabel,
		ct.progressBar,
		container.NewHBox(ct.statusLabel, ct.timeLabel),
	)

	progressCard := widget.NewCard("Progress",
		"Real-time crawling progress and status", progressContent)

	// Statistics section
	statsGrid := container.NewGridWithColumns(2,
		ct.processedLabel, ct.successLabel,
		ct.failedLabel, ct.tokensLabel,
		ct.rateLabel, widget.NewLabel(""),
	)

	statsCard := widget.NewCard("Real-time Statistics",
		"Live crawler performance metrics", statsGrid)

	// Performance monitoring
	performanceCard := ct.createPerformanceCard()

	// Configuration summary
	configCard := ct.createConfigSummaryCard()

	// Recent activity preview
	activityCard := ct.createActivityCard()

	// Control tips and help
	helpCard := ct.createHelpCard()

	// Left column
	leftColumn := container.NewVBox(
		controlCard,
		progressCard,
		statsCard,
	)

	// Right column
	rightColumn := container.NewVBox(
		performanceCard,
		configCard,
		activityCard,
		helpCard,
	)

	// Main layout
	content := container.NewHSplit(leftColumn, rightColumn)
	content.SetOffset(0.5) // 50-50 split

	return content
}

// createPerformanceCard creates the performance monitoring card
func (ct *ControlTab) createPerformanceCard() *widget.Card {
	cpuLabel := widget.NewLabel("CPU: Monitoring...")
	memoryLabel := widget.NewLabel("Memory: 0 MB")
	goroutinesLabel := widget.NewLabel("Goroutines: 0")
	connectionsLabel := widget.NewLabel("Connections: Idle")

	// Start performance monitoring
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Update memory stats
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				memoryMB := m.Alloc / 1024 / 1024
				memoryLabel.SetText(fmt.Sprintf("Memory: %d MB", memoryMB))

				// Update goroutines
				numGoroutines := runtime.NumGoroutine()
				goroutinesLabel.SetText(fmt.Sprintf("Goroutines: %d", numGoroutines))

				// Update connection status
				if ct.gui.isRunning {
					connectionsLabel.SetText("Connections: Active")
				} else {
					connectionsLabel.SetText("Connections: Idle")
				}

				// Update CPU (simplified - would need proper CPU monitoring)
				if ct.gui.isRunning {
					cpuLabel.SetText("CPU: Active")
				} else {
					cpuLabel.SetText("CPU: Idle")
				}

			case <-ct.gui.ctx.Done():
				return
			}
		}
	}()

	performanceGrid := container.NewVBox(
		cpuLabel,
		memoryLabel,
		goroutinesLabel,
		connectionsLabel,
	)

	return widget.NewCard("Performance Monitor",
		"System resource usage and crawler health", performanceGrid)
}

// createConfigSummaryCard creates the configuration summary card
func (ct *ControlTab) createConfigSummaryCard() *widget.Card {
	// This will be updated dynamically
	summaryLabel := widget.NewRichText()

	// Update function
	updateSummary := func() {
		config := ct.gui.configTab.config
		accountCount := len(ct.gui.accountsTab.accounts)
		emailCount := len(ct.gui.emailsTab.emails)

		summary := fmt.Sprintf(`**Current Configuration:**

**Performance:**
- Max Concurrency: %d
- Requests/Second: %.1f
- Request Timeout: %s

**Token Management:**
- Min Tokens: %d
- Max Tokens: %d

**Data:**
- Accounts: %d
- Target Emails: %d

**Status:** %s`,
			config.MaxConcurrency,
			config.RequestsPerSec,
			config.RequestTimeout,
			config.MinTokens,
			config.MaxTokens,
			accountCount,
			emailCount,
			ct.getRunningStatus(),
		)

		summaryLabel.ParseMarkdown(summary)
	}

	// Initial update
	updateSummary()

	// Update periodically
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				updateSummary()
			case <-ct.gui.ctx.Done():
				return
			}
		}
	}()

	return widget.NewCard("Configuration Summary",
		"Active crawler settings and data", summaryLabel)
}

// createActivityCard creates the recent activity card
func (ct *ControlTab) createActivityCard() *widget.Card {
	activityText := widget.NewRichText()
	activityText.ParseMarkdown("*No recent activity*\n\nStart the crawler to see real-time activity updates.")

	// This would be updated with actual crawler activity
	ct.activityText = activityText

	return widget.NewCard("Recent Activity",
		"Last few crawler operations", activityText)
}

// createHelpCard creates the help and tips card
func (ct *ControlTab) createHelpCard() *widget.Card {
	helpInfo := widget.NewRichTextFromMarkdown(`
### 🎮 Control Guide

**Start Crawler:**
- Validates configuration and accounts
- Initializes token extraction
- Begins email processing

**Stop Crawler:**
- Graceful shutdown with state saving
- Exports pending emails
- Preserves partial results

**Monitor Progress:**
- Real-time statistics updates
- Performance metrics tracking
- Error and success counting

### 📊 Understanding Metrics

**Processed:** Total emails handled
**Success:** Emails successfully processed
**Failed:** Emails that failed after retries
**Rate:** Current processing speed
**Tokens:** Available authentication tokens

### 💡 Tips

- Monitor memory usage during large batches
- Watch for rate limiting (429 errors)
- Check token count regularly
- Save configuration before starting
	`)

	return widget.NewCard("Control Help", "", helpInfo)
}

// StartCrawler starts the crawling process
func (ct *ControlTab) StartCrawler() {
	ct.gui.startCrawler()
}

// StopCrawler stops the crawling process
func (ct *ControlTab) StopCrawler() {
	ct.gui.stopCrawler()
}

// PauseCrawler pauses the crawling process
func (ct *ControlTab) PauseCrawler() {
	// Note: Pause functionality would need to be implemented in the crawler core
	ct.gui.updateStatus("Pause functionality coming soon")
	ct.gui.logsTab.AddLog("⏸️ Pause requested (feature coming soon)")
}

// ResumeCrawler resumes the crawling process
func (ct *ControlTab) ResumeCrawler() {
	// Note: Resume functionality would need to be implemented in the crawler core
	ct.gui.updateStatus("Resume functionality coming soon")
	ct.gui.logsTab.AddLog("▶️ Resume requested (feature coming soon)")
}

// OnCrawlerStarted updates UI when crawler starts
func (ct *ControlTab) OnCrawlerStarted() {
	ct.updateButtonStates(true)
	ct.statusLabel.SetText("Status: Starting...")
	ct.startTime = time.Now()
	ct.totalEmails = len(ct.gui.emailsTab.emails)
	ct.processedEmails = 0

	// Reset progress
	ct.progressBar.SetValue(0)
	ct.progressLabel.SetText("Initializing crawler...")

	// Start progress updates
	ct.startProgressUpdates()

	// Update activity
	ct.updateActivity("🚀 Crawler started successfully")
}

// OnCrawlerStopped updates UI when crawler stops
func (ct *ControlTab) OnCrawlerStopped() {
	ct.updateButtonStates(false)
	ct.statusLabel.SetText("Status: Stopped")

	// Stop progress updates
	ct.stopProgressUpdates()

	// Final progress update
	if ct.totalEmails > 0 {
		progress := float64(ct.processedEmails) / float64(ct.totalEmails)
		ct.progressBar.SetValue(progress)
		ct.progressLabel.SetText(fmt.Sprintf("Completed: %d/%d (%.1f%%)",
			ct.processedEmails, ct.totalEmails, progress*100))
	}

	// Update activity
	ct.updateActivity("⏹️ Crawler stopped")
}

// updateButtonStates updates button enabled/disabled states
func (ct *ControlTab) updateButtonStates(running bool) {
	if running {
		ct.startBtn.Disable()
		ct.stopBtn.Enable()
		ct.pauseBtn.Enable()
		ct.resumeBtn.Disable()
	} else {
		ct.startBtn.Enable()
		ct.stopBtn.Disable()
		ct.pauseBtn.Disable()
		ct.resumeBtn.Disable()
	}
}

// startProgressUpdates starts the progress update ticker
func (ct *ControlTab) startProgressUpdates() {
	if ct.updateTicker != nil {
		ct.updateTicker.Stop()
	}

	ct.updateTicker = time.NewTicker(1 * time.Second)

	go func() {
		defer ct.updateTicker.Stop()

		for {
			select {
			case <-ct.updateTicker.C:
				ct.updateProgress()
			case <-ct.gui.ctx.Done():
				return
			}
		}
	}()
}

// stopProgressUpdates stops the progress update ticker
func (ct *ControlTab) stopProgressUpdates() {
	if ct.updateTicker != nil {
		ct.updateTicker.Stop()
		ct.updateTicker = nil
	}
}

// updateProgress updates the progress display
func (ct *ControlTab) updateProgress() {
	if !ct.gui.isRunning {
		return
	}

	// Update time
	elapsed := time.Since(ct.startTime)
	ct.timeLabel.SetText(fmt.Sprintf("Time: %s", ct.formatDuration(elapsed)))

	// Get stats from crawler if available
	ct.gui.crawlerMux.RLock()
	crawler := ct.gui.autoCrawler
	ct.gui.crawlerMux.RUnlock()

	if crawler != nil {
		// Try to get stats from the crawler
		emailStorage, _, _ := crawler.GetStorageServices()
		if emailStorage != nil {
			if stats, err := emailStorage.GetEmailStats(); err == nil {
				processed := stats["success"] + stats["failed"]
				success := stats["success"]
				failed := stats["failed"]
				hasInfo := stats["has_info"]
				noInfo := stats["no_info"]

				ct.processedEmails = processed

				// Update labels
				ct.processedLabel.SetText(fmt.Sprintf("Processed: %d", processed))
				ct.successLabel.SetText(fmt.Sprintf("Success: %d (Data: %d, NoData: %d)", success, hasInfo, noInfo))
				ct.failedLabel.SetText(fmt.Sprintf("Failed: %d", failed))

				// Update progress bar
				if ct.totalEmails > 0 {
					progress := float64(processed) / float64(ct.totalEmails)
					ct.progressBar.SetValue(progress)

					remaining := ct.totalEmails - processed
					ct.progressLabel.SetText(fmt.Sprintf("Progress: %d/%d (%.1f%%) - %d remaining",
						processed, ct.totalEmails, progress*100, remaining))
				}

				// Calculate rate
				if elapsed.Seconds() > 0 {
					rate := float64(processed) / elapsed.Seconds()
					ct.rateLabel.SetText(fmt.Sprintf("Rate: %.2f emails/s", rate))

					// Estimate completion time
					if rate > 0 && ct.totalEmails > processed {
						remaining := ct.totalEmails - processed
						estimatedSeconds := float64(remaining) / rate
						estimatedDuration := time.Duration(estimatedSeconds) * time.Second
						ct.updateActivity(fmt.Sprintf("📊 ETA: %s", ct.formatDuration(estimatedDuration)))
					}
				}
			}
		}

		// Get token info from crawler instance
		crawlerInstance := crawler.GetCrawler()
		if crawlerInstance != nil {
			validTokens := 0
			totalTokens := len(crawlerInstance.Tokens)

			for _, token := range crawlerInstance.Tokens {
				if !crawlerInstance.InvalidTokens[token] {
					validTokens++
				}
			}

			ct.tokensLabel.SetText(fmt.Sprintf("Tokens: %d/%d valid", validTokens, totalTokens))

			// Check crawler status
			if crawlerInstance.AllTokensFailed {
				ct.statusLabel.SetText("Status: Waiting for tokens")
				ct.updateActivity("⚠️ All tokens failed - extracting new tokens")
			} else {
				activeRequests := atomic.LoadInt32(&crawlerInstance.ActiveRequests)
				ct.statusLabel.SetText(fmt.Sprintf("Status: Running (%d active requests)", activeRequests))

				// Update activity based on token status
				if validTokens < 3 {
					ct.updateActivity(fmt.Sprintf("⚠️ Low token count: %d remaining", validTokens))
				}
			}
		}
	}
}

// updateActivity updates the activity display
func (ct *ControlTab) updateActivity(message string) {
	if ct.activityText != nil {
		timestamp := time.Now().Format("15:04:05")
		activity := fmt.Sprintf("[%s] %s", timestamp, message)

		// Keep only recent activities (last 10)
		currentText := ct.activityText.String()
		lines := strings.Split(currentText, "\n")

		// Add new activity
		lines = append(lines, activity)

		// Keep only last 10 lines
		if len(lines) > 10 {
			lines = lines[len(lines)-10:]
		}

		newText := "**Recent Activity:**\n\n" + strings.Join(lines, "\n")
		ct.activityText.ParseMarkdown(newText)
	}
}

// formatDuration formats a duration for display
func (ct *ControlTab) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	} else {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
}

// getRunningStatus returns the current running status as string
func (ct *ControlTab) getRunningStatus() string {
	if ct.gui.isRunning {
		return "Running"
	}
	return "Stopped"
}

// Additional helper methods for the control tab

// GetCurrentStats returns current crawler statistics
func (ct *ControlTab) GetCurrentStats() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["running"] = ct.gui.isRunning
	stats["total_emails"] = ct.totalEmails
	stats["processed_emails"] = ct.processedEmails

	if !ct.startTime.IsZero() {
		stats["elapsed_time"] = time.Since(ct.startTime).Seconds()
	}

	// Get detailed stats from crawler if available
	ct.gui.crawlerMux.RLock()
	crawler := ct.gui.autoCrawler
	ct.gui.crawlerMux.RUnlock()

	if crawler != nil {
		emailStorage, _, _ := crawler.GetStorageServices()
		if emailStorage != nil {
			if dbStats, err := emailStorage.GetEmailStats(); err == nil {
				for key, value := range dbStats {
					stats[key] = value
				}
			}
		}

		crawlerInstance := crawler.GetCrawler()
		if crawlerInstance != nil {
			stats["total_tokens"] = len(crawlerInstance.Tokens)

			validTokens := 0
			for _, token := range crawlerInstance.Tokens {
				if !crawlerInstance.InvalidTokens[token] {
					validTokens++
				}
			}
			stats["valid_tokens"] = validTokens
			stats["active_requests"] = atomic.LoadInt32(&crawlerInstance.ActiveRequests)
		}
	}

	return stats
}

// Emergency stop function for critical situations
func (ct *ControlTab) EmergencyStop() {
	ct.gui.logsTab.AddLog("🆘 Emergency stop initiated")
	ct.StopCrawler()

	// Force cleanup if normal stop doesn't work
	go func() {
		time.Sleep(5 * time.Second)
		if ct.gui.isRunning {
			ct.gui.logsTab.AddLog("🔴 Force stopping crawler...")
			ct.gui.cleanup()
		}
	}()
}

// Export current session statistics
func (ct *ControlTab) ExportSessionStats() {
	stats := ct.GetCurrentStats()

	// Create stats report
	report := fmt.Sprintf(`# Crawler Session Statistics
Generated: %s

## Session Info
- Status: %s
- Start Time: %s
- Duration: %s

## Processing Stats
- Total Emails: %v
- Processed: %v
- Success: %v
- Failed: %v
- Has LinkedIn Info: %v
- No LinkedIn Info: %v

## Performance
- Processing Rate: %.2f emails/second
- Valid Tokens: %v/%v
- Active Requests: %v

## System Resources
- Memory Usage: %d MB
- Goroutines: %d
`,
		time.Now().Format("2006-01-02 15:04:05"),
		ct.getRunningStatus(),
		ct.startTime.Format("15:04:05"),
		ct.formatDuration(time.Since(ct.startTime)),
		stats["total_emails"],
		stats["processed_emails"],
		stats["success"],
		stats["failed"],
		stats["has_info"],
		stats["no_info"],
		func() float64 {
			if elapsed := time.Since(ct.startTime).Seconds(); elapsed > 0 {
				return float64(ct.processedEmails) / elapsed
			}
			return 0.0
		}(),
		stats["valid_tokens"],
		stats["total_tokens"],
		stats["active_requests"],
		func() uint64 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			return m.Alloc / 1024 / 1024
		}(),
		runtime.NumGoroutine(),
	)

	// Chỉ cần thêm 1 dòng như sau để không lỗi:
	ct.gui.logsTab.AddLog(report)
	// Hoặc hiển thị dialog
	// dialog.ShowInformation("Session Stats", report, ct.gui.window)

	ct.gui.updateStatus("Session statistics exported")
}
