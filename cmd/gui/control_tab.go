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

	// Recent activity
	activityCard := ct.createActivityCard()

	// Left column with better spacing
	leftColumn := container.NewVBox(
		controlCard,
		widget.NewSeparator(),
		progressCard,
		widget.NewSeparator(),
		statsCard,
	)

	// Right column with better spacing
	rightColumn := container.NewVBox(
		performanceCard,
		widget.NewSeparator(),
		activityCard,
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

	// Không dùng go func! Sử dụng time.Ticker trong main thread bằng callback Update UI.
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
	// Tạo ticker bằng time.NewTicker, nhưng **callback gọi updateFunc trực tiếp từ main thread**:
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Thực thi updateFunc trên main thread bằng cách gọi qua channel
				fyne.CurrentApp().SendNotification(&fyne.Notification{
					Title:   "",
					Content: "",
				}) // Đánh thức main event loop, không update UI ở đây!
				ct.gui.updateUI <- updateFunc
			case <-ct.gui.ctx.Done():
				return
			}
		}
	}()
	// Gọi lần đầu ngay khi khởi tạo
	updateFunc()

	performanceGrid := container.NewVBox(
		memoryLabel,
		goroutinesLabel,
		connectionsLabel,
	)

	return widget.NewCard("Performance", "", performanceGrid)
}

// createActivityCard creates the recent activity card
func (ct *ControlTab) createActivityCard() *widget.Card {
	activityText := widget.NewRichText()
	activityText.ParseMarkdown("*Ready to start crawling*")

	ct.activityText = activityText

	return widget.NewCard("Activity", "", activityText)
}

// StartCrawler starts the crawling process
func (ct *ControlTab) StartCrawler() {
	ct.gui.startCrawler()
}

// StopCrawler stops the crawling process
func (ct *ControlTab) StopCrawler() {
	ct.gui.stopCrawler()
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
	ct.updateTicker = time.NewTicker(2 * time.Second)
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
				ct.statusLabel.SetText(fmt.Sprintf("Status: Running (%d active)", activeRequests))

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

		// Keep only recent activities (last 5)
		currentText := ct.activityText.String()
		lines := strings.Split(currentText, "\n")

		// Add new activity
		lines = append(lines, activity)

		// Keep only last 5 lines
		if len(lines) > 5 {
			lines = lines[len(lines)-5:]
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
