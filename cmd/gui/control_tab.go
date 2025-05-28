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
		gui:            gui,
		activityBuffer: []string{},
	}

	// Initialize buttons
	tab.startBtn = widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), tab.StartCrawler)
	tab.stopBtn = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), tab.StopCrawler)

	// Style buttons
	tab.startBtn.Importance = widget.HighImportance
	tab.stopBtn.Importance = widget.DangerImportance

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
	// Control buttons section with better spacing
	controlButtons := container.NewHBox(
		ct.startBtn,
		widget.NewSeparator(),
		ct.stopBtn,
	)

	controlCard := widget.NewCard("Control", "",
		container.NewVBox(
			controlButtons,
			widget.NewSeparator(),
		))

	// Progress section with better spacing
	progressContent := container.NewVBox(
		ct.progressLabel,
		ct.progressBar,
		widget.NewSeparator(),
		container.NewHBox(ct.statusLabel, ct.timeLabel),
	)

	progressCard := widget.NewCard("Progress", "", progressContent)

	// Statistics section with better spacing
	statsGrid := container.NewVBox(
		ct.processedLabel,
		widget.NewSeparator(),
		ct.successLabel,
		ct.failedLabel,
		widget.NewSeparator(),
		ct.tokensLabel,
		ct.rateLabel,
	)

	statsCard := widget.NewCard("Statistics", "", statsGrid)

	// Performance monitoring
	performanceCard := ct.createPerformanceCard()

	// Recent activity - MỞ RỘNG ACTIVITY LOG
	activityCard := ct.createActivityCard()

	// Left column with better spacing
	leftColumn := container.NewVBox(
		controlCard,
		widget.NewSeparator(),
		progressCard,
		widget.NewSeparator(),
		statsCard,
	)

	// Right column - Activity log mở rộng xuống dưới
	rightColumn := container.NewBorder(
		performanceCard,
		nil, nil, nil,
		activityCard, // Activity card chiếm phần lớn không gian
	)

	// Main layout
	content := container.NewHSplit(leftColumn, rightColumn)
	content.SetOffset(0.5) // 50-50 split

	return content
}

// createPerformanceCard creates the performance monitoring card
func (ct *ControlTab) createPerformanceCard() *widget.Card {
	memoryLabel := widget.NewLabel("Memory: 0 MB")
	goroutinesLabel := widget.NewLabel("Goroutines: 0")
	connectionsLabel := widget.NewLabel("Status: Idle")

	// Performance update function
	updateFunc := func() {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		memoryMB := m.Alloc / 1024 / 1024
		memoryLabel.SetText(fmt.Sprintf("Memory: %d MB", memoryMB))

		// Update goroutines
		numGoroutines := runtime.NumGoroutine()
		goroutinesLabel.SetText(fmt.Sprintf("Goroutines: %d", numGoroutines))

		// Update connection status
		if ct.gui.isRunning {
			connectionsLabel.SetText("Status: Running")
		} else {
			connectionsLabel.SetText("Status: Idle")
		}
	}

	// Start performance monitoring
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ct.gui.updateUI <- updateFunc
			case <-ct.gui.ctx.Done():
				return
			}
		}
	}()

	// Initial update
	updateFunc()

	performanceGrid := container.NewVBox(
		memoryLabel,
		goroutinesLabel,
		connectionsLabel,
	)

	return widget.NewCard("Performance", "", performanceGrid)
}

// createActivityCard creates the recent activity card - MỞ RỘNG
func (ct *ControlTab) createActivityCard() *widget.Card {
	ct.activityText = widget.NewRichText()
	ct.activityText.ParseMarkdown("*Ready to start crawling*")
	ct.activityText.Wrapping = fyne.TextWrapWord

	// Use scroll container for activity log
	activityScroll := container.NewScroll(ct.activityText)

	return widget.NewCard("Activity Log", "", activityScroll)
}

// StartCrawler starts the crawling process - INTEGRATE WITH MAIN GUI
func (ct *ControlTab) StartCrawler() {
	// Use the main GUI's start crawler function
	ct.gui.startCrawler()
}

// StopCrawler stops the crawling process - INTEGRATE WITH MAIN GUI
func (ct *ControlTab) StopCrawler() {
	// Use the main GUI's stop crawler function
	ct.gui.stopCrawler()
}

// OnCrawlerStarted updates UI when crawler starts
func (ct *ControlTab) OnCrawlerStarted() {
	ct.updateButtonStates(true)
	ct.statusLabel.SetText("Status: Starting...")
	ct.startTime = time.Now()

	// Get total emails from emails tab
	if ct.gui.emailsTab != nil {
		ct.totalEmails = len(ct.gui.emailsTab.GetEmails())
	}
	ct.processedEmails = 0

	// Reset progress
	ct.progressBar.SetValue(0)
	ct.progressLabel.SetText("Initializing...")

	// Start progress updates
	ct.startProgressUpdates()

	// Update activity
	ct.updateActivity("🚀 Crawler started")
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
	} else {
		ct.startBtn.Enable()
		ct.stopBtn.Disable()
	}
}

// startProgressUpdates starts the progress update ticker
func (ct *ControlTab) startProgressUpdates() {
	if ct.updateTicker != nil {
		ct.updateTicker.Stop()
	}
	ct.updateTicker = time.NewTicker(3 * time.Second) // Update every 3 seconds
	go func() {
		defer ct.updateTicker.Stop()
		for {
			select {
			case <-ct.updateTicker.C:
				ct.gui.updateUI <- func() {
					ct.updateProgress()
				}
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

	// Get stats from the active crawler
	ct.gui.crawlerMux.RLock()
	autoCrawler := ct.gui.autoCrawler
	ct.gui.crawlerMux.RUnlock()

	if autoCrawler != nil {
		// Get stats from SQLite database
		emailStorage, _, _ := autoCrawler.GetStorageServices()
		if emailStorage != nil {
			if stats, err := emailStorage.GetEmailStats(); err == nil {
				processed := stats["success"] + stats["failed"]
				success := stats["success"]
				failed := stats["failed"]
				hasInfo := stats["has_info"]
				noInfo := stats["no_info"]
				pending := stats["pending"]

				ct.processedEmails = processed

				// Update labels
				ct.processedLabel.SetText(fmt.Sprintf("Processed: %d", processed))
				ct.successLabel.SetText(fmt.Sprintf("Success: %d (LinkedIn: %d, NoData: %d)", success, hasInfo, noInfo))
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
				}

				// Update activity with important events
				if processed > 0 && processed%25 == 0 {
					ct.updateActivity(fmt.Sprintf("📊 Processed %d emails (%.1f%% complete)",
						processed, float64(processed)*100/float64(ct.totalEmails)))
				}

				if hasInfo > 0 && hasInfo%5 == 0 {
					ct.updateActivity(fmt.Sprintf("🎯 Found %d LinkedIn profiles!", hasInfo))
				}

				// Log token extraction progress
				if pending > 0 && processed == 0 {
					ct.updateActivity("🔑 Extracting tokens from accounts...")
				}
			}
		}

		// Get token info from crawler instance
		crawlerInstance := autoCrawler.GetCrawler()
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
				ct.statusLabel.SetText(fmt.Sprintf("Status: Running (%d active)", activeRequests))

				// Update activity based on token status
				if validTokens < 3 && validTokens > 0 {
					ct.updateActivity(fmt.Sprintf("⚠️ Low token count: %d remaining", validTokens))
				}

				// Activity for token refresh
				if totalTokens > 0 && validTokens == 0 {
					ct.updateActivity("🔄 Refreshing tokens...")
				}
			}
		} else {
			// No crawler instance yet - probably initializing
			ct.statusLabel.SetText("Status: Initializing...")
			ct.updateActivity("🔧 Setting up crawler components...")
		}
	}
}

// updateActivity updates the activity display - MỞ RỘNG ACTIVITY LOG
func (ct *ControlTab) updateActivity(message string) {
	if ct.activityText != nil {
		timestamp := time.Now().Format("15:04:05")
		activity := fmt.Sprintf("[%s] %s", timestamp, message)

		// Keep activity history
		if ct.activityBuffer == nil {
			ct.activityBuffer = []string{}
		}

		// Add new activity
		ct.activityBuffer = append(ct.activityBuffer, activity)

		// Keep only last 100 lines to prevent memory issues
		if len(ct.activityBuffer) > 100 {
			ct.activityBuffer = ct.activityBuffer[len(ct.activityBuffer)-100:]
		}

		// Create markdown content with all activities
		var content strings.Builder
		content.WriteString("**Recent Activity:**\n\n")
		content.WriteString("```\n")
		for _, line := range ct.activityBuffer {
			content.WriteString(line)
			content.WriteString("\n")
		}
		content.WriteString("```")

		ct.activityText.ParseMarkdown(content.String())
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

// Additional helper methods for better integration

// GetProgressInfo returns current progress information
func (ct *ControlTab) GetProgressInfo() (processed, total int, rate float64) {
	return ct.processedEmails, ct.totalEmails, ct.getCurrentRate()
}

// getCurrentRate calculates current processing rate
func (ct *ControlTab) getCurrentRate() float64 {
	if ct.startTime.IsZero() {
		return 0.0
	}

	elapsed := time.Since(ct.startTime).Seconds()
	if elapsed > 0 {
		return float64(ct.processedEmails) / elapsed
	}
	return 0.0
}

// AddCustomActivity allows other components to add activity messages
func (ct *ControlTab) AddCustomActivity(message string) {
	ct.gui.updateUI <- func() {
		ct.updateActivity(message)
	}
}

// ResetProgress resets all progress indicators
func (ct *ControlTab) ResetProgress() {
	ct.processedEmails = 0
	ct.totalEmails = 0
	ct.startTime = time.Time{}

	ct.progressBar.SetValue(0)
	ct.progressLabel.SetText("Ready to start")
	ct.statusLabel.SetText("Status: Ready")
	ct.timeLabel.SetText("Time: 00:00:00")

	ct.processedLabel.SetText("Processed: 0")
	ct.successLabel.SetText("Success: 0")
	ct.failedLabel.SetText("Failed: 0")
	ct.tokensLabel.SetText("Tokens: 0")
	ct.rateLabel.SetText("Rate: 0.0/s")
}

// UpdateTokenInfo updates token information display
func (ct *ControlTab) UpdateTokenInfo(valid, total int) {
	ct.gui.updateUI <- func() {
		ct.tokensLabel.SetText(fmt.Sprintf("Tokens: %d/%d valid", valid, total))
		if valid == 0 && total > 0 {
			ct.updateActivity("❌ No valid tokens available")
		} else if valid < 3 && valid > 0 {
			ct.updateActivity(fmt.Sprintf("⚠️ Low token count: %d valid", valid))
		}
	}
}

// SetCrawlerStatus updates the crawler status display
func (ct *ControlTab) SetCrawlerStatus(status string) {
	ct.gui.updateUI <- func() {
		ct.statusLabel.SetText(fmt.Sprintf("Status: %s", status))
		ct.updateActivity(fmt.Sprintf("ℹ️ Status: %s", status))
	}
}

// ShowError displays an error in the activity log
func (ct *ControlTab) ShowError(err error) {
	ct.gui.updateUI <- func() {
		ct.updateActivity(fmt.Sprintf("❌ Error: %v", err))
	}
}

// ShowSuccess displays a success message in the activity log
func (ct *ControlTab) ShowSuccess(message string) {
	ct.gui.updateUI <- func() {
		ct.updateActivity(fmt.Sprintf("✅ %s", message))
	}
}

// ShowWarning displays a warning message in the activity log
func (ct *ControlTab) ShowWarning(message string) {
	ct.gui.updateUI <- func() {
		ct.updateActivity(fmt.Sprintf("⚠️ %s", message))
	}
}
