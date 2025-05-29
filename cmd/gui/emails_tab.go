package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"linkedin-crawler/internal/config"
	"linkedin-crawler/internal/orchestrator"
	storageInternal "linkedin-crawler/internal/storage"
	"linkedin-crawler/internal/utils"
)

type EmailsTab struct {
	gui           *CrawlerGUI
	emailsList    *widget.List
	emails        []string
	emailData     binding.StringList
	importBtn     *widget.Button
	clearBtn      *widget.Button
	startCrawlBtn *widget.Button
	stopCrawlBtn  *widget.Button

	logText   *widget.RichText
	logBuffer []string

	totalLabel    *widget.Label
	pendingLabel  *widget.Label
	successLabel  *widget.Label
	failedLabel   *widget.Label
	hasInfoLabel  *widget.Label
	noInfoLabel   *widget.Label
	progressBar   *widget.ProgressBar
	progressLabel *widget.Label
	statusLabel   *widget.Label
	selectedIndex int

	// Crawling state
	isCrawling  int32 // atomic flag
	crawlCancel context.CancelFunc
	autoCrawler *orchestrator.AutoCrawler

	// Email status cache để tránh query database liên tục
	emailStatusCache map[string]string
	lastCacheUpdate  time.Time

	// Stats refresh ticker
	statsRefreshTicker *time.Ticker
	lastStats          map[string]int // Cache stats để tránh reset về 0

	// OPTIMIZATION: Virtual scrolling và pagination
	displayEmails    []string // Emails hiển thị trong UI (limited)
	totalEmailCount  int      // Tổng số emails thực tế
	currentPage      int      // Trang hiện tại
	emailsPerPage    int      // Emails per page
	maxDisplayEmails int      // Tối đa emails hiển thị trong UI

	// Performance monitoring
	lastUpdateTime time.Time
	updateCount    int32

	// Page info update function
	updatePageInfo func()
}

func NewEmailsTab(gui *CrawlerGUI) *EmailsTab {
	tab := &EmailsTab{
		gui:              gui,
		emails:           []string{},
		emailData:        binding.NewStringList(),
		emailStatusCache: make(map[string]string),
		lastCacheUpdate:  time.Time{},
		lastStats:        make(map[string]int),

		// OPTIMIZATION: Pagination settings
		emailsPerPage:    1000, // 1000 emails per page
		maxDisplayEmails: 5000, // Max 5000 emails in UI at once
		currentPage:      0,
		displayEmails:    []string{},
	}

	tab.importBtn = widget.NewButtonWithIcon("Import", theme.FolderOpenIcon(), tab.ImportEmails)
	tab.clearBtn = widget.NewButtonWithIcon("Clear All", theme.DeleteIcon(), tab.ClearAllEmails)
	tab.clearBtn.Importance = widget.DangerImportance

	tab.startCrawlBtn = widget.NewButtonWithIcon("Start Crawl", theme.MediaPlayIcon(), tab.StartCrawl)
	tab.stopCrawlBtn = widget.NewButtonWithIcon("Stop Crawl", theme.MediaStopIcon(), tab.StopCrawl)
	tab.stopCrawlBtn.Importance = widget.DangerImportance
	tab.stopCrawlBtn.Disable() // Initially disabled

	tab.logText = widget.NewRichText()
	tab.logText.Wrapping = fyne.TextWrapWord
	tab.logBuffer = []string{}

	tab.totalLabel = widget.NewLabel("Total: 0")
	tab.pendingLabel = widget.NewLabel("Pending: 0")
	tab.successLabel = widget.NewLabel("Success: 0")
	tab.failedLabel = widget.NewLabel("Failed: 0")
	tab.hasInfoLabel = widget.NewLabel("Has LinkedIn: 0")
	tab.noInfoLabel = widget.NewLabel("No LinkedIn: 0")
	tab.progressBar = widget.NewProgressBar()
	tab.progressLabel = widget.NewLabel("Ready")
	tab.statusLabel = widget.NewLabel("Status: Ready")

	tab.setupEmailsList()

	// Start stats refresh ticker with throttling
	tab.startStatsRefresh()

	return tab
}

func (et *EmailsTab) CreateContent() fyne.CanvasObject {
	fileButtons := container.NewHBox(
		et.importBtn,
		et.clearBtn,
		widget.NewButton("Refresh", et.RefreshEmailsList),
	)

	// OPTIMIZATION: Add pagination controls
	prevPageBtn := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
		if et.currentPage > 0 {
			et.currentPage--
			et.updateDisplayEmails()
		}
	})

	nextPageBtn := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() {
		maxPages := et.getTotalPages()
		if et.currentPage < maxPages-1 {
			et.currentPage++
			et.updateDisplayEmails()
		}
	})

	pageInfoLabel := widget.NewLabel("Page: 1/1")
	et.updatePageInfo = func() {
		maxPages := et.getTotalPages()
		if maxPages == 0 {
			pageInfoLabel.SetText("Page: 0/0")
		} else {
			pageInfoLabel.SetText(fmt.Sprintf("Page: %d/%d", et.currentPage+1, maxPages))
		}

		prevPageBtn.Enable()
		nextPageBtn.Enable()
		if et.currentPage <= 0 {
			prevPageBtn.Disable()
		}
		if et.currentPage >= maxPages-1 {
			nextPageBtn.Disable()
		}
	}

	paginationControls := container.NewHBox(
		prevPageBtn,
		pageInfoLabel,
		nextPageBtn,
		widget.NewSeparator(),
		widget.NewLabel("Showing:"),
		widget.NewLabel("1000/page"),
	)

	statsRow1 := container.NewHBox(
		et.totalLabel,
		widget.NewSeparator(),
		et.pendingLabel,
		widget.NewSeparator(),
		et.successLabel,
	)
	statsRow2 := container.NewHBox(
		et.failedLabel,
		widget.NewSeparator(),
		et.hasInfoLabel,
		widget.NewSeparator(),
		et.noInfoLabel,
	)
	statsGrid := container.NewVBox(statsRow1, statsRow2)

	leftPanel := container.NewVBox(
		widget.NewCard("File Operations", "", fileButtons),
		widget.NewCard("Statistics", "", statsGrid),
		widget.NewCard("Pagination", "", paginationControls), // NEW: Pagination controls
		container.NewScroll(et.emailsList),
	)

	// Control buttons
	controlButtons := container.NewVBox(
		et.startCrawlBtn,
		et.stopCrawlBtn,
	)

	// Progress section
	progressSection := container.NewVBox(
		et.progressLabel,
		et.progressBar,
		et.statusLabel,
	)

	// Log area - MỞ RỘNG XUỐNG DƯỚI
	logScroll := container.NewScroll(et.logText)
	logArea := container.NewBorder(
		widget.NewLabel("Email Crawl Log:"), nil, nil, nil,
		logScroll,
	)

	// Right panel with expanded log area
	rightPanel := container.NewBorder(
		container.NewVBox(
			widget.NewCard("Email Crawl Control", "", controlButtons),
			widget.NewCard("Progress", "", progressSection),
		),
		nil, nil, nil,
		widget.NewCard("Logs", "", logArea), // Log area chiếm phần lớn không gian
	)

	content := container.NewHSplit(leftPanel, rightPanel)
	content.SetOffset(0.5) // 50-50 split
	return content
}

// OPTIMIZATION: Virtual scrolling với pagination
func (et *EmailsTab) getTotalPages() int {
	if et.emailsPerPage <= 0 {
		return 1
	}
	totalPages := (et.totalEmailCount + et.emailsPerPage - 1) / et.emailsPerPage
	if totalPages == 0 {
		return 1
	}
	return totalPages
}

// OPTIMIZATION: Update display emails for current page
func (et *EmailsTab) updateDisplayEmails() {
	if len(et.emails) == 0 {
		et.displayEmails = []string{}
		et.updateEmailsList()
		et.updatePageInfo()
		return
	}

	start := et.currentPage * et.emailsPerPage
	end := start + et.emailsPerPage

	if start >= len(et.emails) {
		et.currentPage = 0
		start = 0
		end = et.emailsPerPage
	}

	if end > len(et.emails) {
		end = len(et.emails)
	}

	et.displayEmails = et.emails[start:end]
	et.updateEmailsList()
	et.updatePageInfo()
}

// OPTIMIZATION: Update emails list với limited items
func (et *EmailsTab) updateEmailsList() {
	// Clear existing data
	et.emailData = binding.NewStringList()

	// Add only display emails (max 1000 per page)
	for _, email := range et.displayEmails {
		et.emailData.Append(email)
	}

	et.emailsList.Refresh()
}

func (et *EmailsTab) setupEmailsList() {
	et.emailsList = widget.NewListWithData(
		et.emailData,
		func() fyne.CanvasObject {
			icon := widget.NewIcon(theme.MailSendIcon())
			email := widget.NewLabel("Email")
			status := widget.NewLabel("Status")
			return container.NewHBox(icon, container.NewVBox(email, status))
		},
		func(id binding.DataItem, obj fyne.CanvasObject) {
			str, _ := id.(binding.String).Get()
			container := obj.(*fyne.Container)
			icon := container.Objects[0].(*widget.Icon)
			infoContainer := container.Objects[1].(*fyne.Container)
			emailLabel := infoContainer.Objects[0].(*widget.Label)
			statusLabel := infoContainer.Objects[1].(*widget.Label)
			emailLabel.SetText(str)

			// OPTIMIZATION: Only get status for visible emails
			status := "Pending"              // Default status - avoid expensive DB queries for all emails
			if len(et.displayEmails) < 100 { // Only get real status for small lists
				status = et.getEmailStatus(str)
			}
			statusLabel.SetText(status)

			// Set appropriate icon based on status
			switch status {
			case "Pending":
				icon.SetResource(theme.MailSendIcon())
			case "Success - Has LinkedIn":
				icon.SetResource(theme.ConfirmIcon())
			case "Success - No LinkedIn":
				icon.SetResource(theme.InfoIcon())
			case "Failed":
				icon.SetResource(theme.ErrorIcon())
			default:
				icon.SetResource(theme.MailSendIcon())
			}
		},
	)
	et.emailsList.OnSelected = func(id widget.ListItemID) {
		if int(id) < len(et.displayEmails) {
			et.selectedIndex = int(id)
		}
	}
}

// OPTIMIZATION: Start stats refresh ticker with throttling
func (et *EmailsTab) startStatsRefresh() {
	if et.statsRefreshTicker != nil {
		et.statsRefreshTicker.Stop()
	}

	et.statsRefreshTicker = time.NewTicker(5 * time.Second) // Slower refresh: 5 seconds
	go func() {
		defer func() {
			if et.statsRefreshTicker != nil {
				et.statsRefreshTicker.Stop()
			}
		}()

		for {
			select {
			case <-et.statsRefreshTicker.C:
				// OPTIMIZATION: Throttle updates to prevent UI lag
				if time.Since(et.lastUpdateTime) > 3*time.Second {
					et.gui.updateUI <- func() {
						et.updateStatsFromDatabase()
						et.lastUpdateTime = time.Now()
						atomic.AddInt32(&et.updateCount, 1)
					}
				}
			case <-et.gui.ctx.Done():
				return
			}
		}
	}()
}

// OPTIMIZATION: Chunked, non-blocking import with progress
func (et *EmailsTab) ImportEmails() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()

		// Show progress dialog with cancel button
		progress := dialog.NewProgressInfinite("Importing", "Reading file...", et.gui.window)
		progress.Show()

		// Process in background thread to avoid blocking UI
		go func() {
			defer progress.Hide()

			startTime := time.Now()

			// OPTIMIZATION: Use streaming reader for large files
			scanner := bufio.NewScanner(reader)
			scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB buffer for huge files

			emailSet := make(map[string]struct{}) // O(1) deduplication
			emails := make([]string, 0, 100000)   // Pre-allocate for performance

			emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

			var totalLines, validEmails, duplicates, invalidEmails int
			chunkSize := 10000 // Process 10k lines at a time

			et.gui.updateUI <- func() {
				progress.Hide()
				progress = dialog.NewProgressInfinite("Processing", "Validating emails...", et.gui.window)
				progress.Show()
			}

			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				totalLines++

				// Skip empty lines and comments
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}

				// Extract email from CSV format
				email := line
				if strings.Contains(line, ",") {
					parts := strings.Split(line, ",")
					email = strings.TrimSpace(parts[len(parts)-1])
				}

				// Validate email format
				if !emailRegex.MatchString(email) {
					invalidEmails++
					continue
				}

				// Check for duplicates
				emailLower := strings.ToLower(email)
				if _, exists := emailSet[emailLower]; exists {
					duplicates++
					continue
				}

				emailSet[emailLower] = struct{}{}
				emails = append(emails, email)
				validEmails++

				// OPTIMIZATION: Update progress periodically and yield to UI
				if totalLines%chunkSize == 0 {
					currentCount := len(emails)
					et.gui.updateUI <- func() {
						progress.Hide()
						progress = dialog.NewProgressInfinite(
							"Processing",
							fmt.Sprintf("Processed %d lines, found %d valid emails...", totalLines, currentCount),
							et.gui.window,
						)
						progress.Show()
					}

					// Small delay to let UI refresh
					time.Sleep(10 * time.Millisecond)
				}
			}

			if err := scanner.Err(); err != nil {
				et.gui.updateUI <- func() {
					progress.Hide()
					dialog.ShowError(fmt.Errorf("Error reading file: %v", err), et.gui.window)
				}
				return
			}

			processingTime := time.Since(startTime)

			// OPTIMIZATION: Update UI with final results
			et.gui.updateUI <- func() {
				// Store all emails but limit UI display
				et.emails = emails
				et.totalEmailCount = len(emails)
				et.currentPage = 0

				// Update display with pagination
				et.updateDisplayEmails()
				et.updateStats()

				progress.Hide()

				// Show detailed results
				message := fmt.Sprintf(
					"Import completed in %.2f seconds!\n\n"+
						"📊 Results:\n"+
						"✅ Valid emails: %s\n"+
						"📝 Total lines processed: %s\n"+
						"🔄 Duplicates skipped: %s\n"+
						"❌ Invalid emails: %s\n\n"+
						"💡 Large dataset detected!\n"+
						"Using pagination: %d emails per page\n"+
						"Current page: 1/%d",
					processingTime.Seconds(),
					et.formatNumber(validEmails),
					et.formatNumber(totalLines),
					et.formatNumber(duplicates),
					et.formatNumber(invalidEmails),
					et.emailsPerPage,
					et.getTotalPages(),
				)

				dialog.ShowInformation("Import Results", message, et.gui.window)
				et.gui.updateStatus(fmt.Sprintf("Imported %s emails (showing page 1/%d)",
					et.formatNumber(validEmails), et.getTotalPages()))
				et.addLog(fmt.Sprintf("📥 Import: %s emails in %.2f seconds",
					et.formatNumber(validEmails), processingTime.Seconds()))
			}
		}()
	}, et.gui.window)
}

// OPTIMIZATION: Format large numbers with commas
func (et *EmailsTab) formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	str := fmt.Sprintf("%d", n)
	result := ""

	for i, char := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(char)
	}

	return result
}

// START CRAWL - Hoạt động thực tế với token priority check
func (et *EmailsTab) StartCrawl() {
	// Check if already running
	if atomic.LoadInt32(&et.isCrawling) == 1 {
		et.addLog("⚠️ Email crawling đã đang chạy!")
		return
	}

	// Check if there are emails
	if len(et.emails) == 0 {
		et.addLog("❌ Không có emails để crawl!")
		dialog.ShowError(fmt.Errorf("Không có emails để crawl"), et.gui.window)
		return
	}

	// OPTIMIZATION: Show confirmation for large datasets
	if len(et.emails) > 100000 {
		dialog.ShowConfirm(
			"Large Dataset Detected",
			fmt.Sprintf("You're about to crawl %s emails.\n\nThis may take several hours to complete.\n\nDo you want to continue?",
				et.formatNumber(len(et.emails))),
			func(confirmed bool) {
				if confirmed {
					et.startCrawlProcess()
				}
			}, et.gui.window)
		return
	}

	et.startCrawlProcess()
}

func (et *EmailsTab) startCrawlProcess() {
	// Check tokens first, then accounts
	if !et.checkTokensAvailability() {
		et.addLog("❌ Không có tokens và không có accounts để lấy tokens!")
		dialog.ShowError(fmt.Errorf("Cần có tokens hoặc accounts để crawl"), et.gui.window)
		return
	}

	// Set running state
	atomic.StoreInt32(&et.isCrawling, 1)
	et.startCrawlBtn.Disable()
	et.stopCrawlBtn.Enable()

	et.addLog(fmt.Sprintf("🚀 Bắt đầu crawl %s emails...", et.formatNumber(len(et.emails))))
	et.addLog(fmt.Sprintf("📊 Estimated time: %s", et.estimateProcessingTime()))

	// Log token/account status
	et.logTokenAccountStatus()

	// Save emails to file first
	et.SaveEmails()

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	et.crawlCancel = cancel

	// Run crawling in background
	go func() {
		defer func() {
			// Reset state when done
			atomic.StoreInt32(&et.isCrawling, 0)
			et.autoCrawler = nil
			et.gui.updateUI <- func() {
				et.startCrawlBtn.Enable()
				et.stopCrawlBtn.Disable()
				et.addLog("✅ Email crawling hoàn thành!")
				// Clear cache to force refresh
				et.clearEmailStatusCache()
				// Update stats from database after completion
				et.updateStatsFromDatabase()
				// Refresh current page
				et.updateDisplayEmails()
				// QUAN TRỌNG: Lưu stats cuối cùng và export pending emails
				et.finalizeAfterStop()
			}
		}()

		et.performEmailCrawling(ctx)
	}()
}

// OPTIMIZATION: Estimate processing time based on email count
func (et *EmailsTab) estimateProcessingTime() string {
	emailCount := len(et.emails)

	// Rough estimate: 15-20 emails/second
	estimatedSeconds := float64(emailCount) / 17.5

	if estimatedSeconds < 60 {
		return fmt.Sprintf("~%.0f seconds", estimatedSeconds)
	} else if estimatedSeconds < 3600 {
		return fmt.Sprintf("~%.0f minutes", estimatedSeconds/60)
	} else {
		hours := estimatedSeconds / 3600
		return fmt.Sprintf("~%.1f hours", hours)
	}
}

// STOP CRAWL - Hoạt động thực tế với lưu trạng thái
func (et *EmailsTab) StopCrawl() {
	if atomic.LoadInt32(&et.isCrawling) == 0 {
		et.addLog("⚠️ Email crawling không đang chạy!")
		return
	}

	et.addLog("⏹️ Đang dừng email crawling...")

	// Signal shutdown to autoCrawler
	if et.autoCrawler != nil {
		shutdownReq := et.autoCrawler.GetShutdownRequested()
		if shutdownReq != nil {
			atomic.StoreInt32(shutdownReq, 1)
		}
	}

	// Cancel the context
	if et.crawlCancel != nil {
		et.crawlCancel()
	}

	// Reset state immediately
	atomic.StoreInt32(&et.isCrawling, 0)
	et.startCrawlBtn.Enable()
	et.stopCrawlBtn.Disable()

	et.addLog("🛑 Đã dừng email crawling!")

	// QUAN TRỌNG: Không clear cache ngay, để giữ lại stats hiện tại
	et.addLog("💾 Đang lưu trạng thái hiện tại...")
	et.finalizeAfterStop()

	// Update stats from database after stopping (with delay to ensure data is saved)
	time.AfterFunc(2*time.Second, func() {
		et.gui.updateUI <- func() {
			et.updateStatsFromDatabase()
			et.updateDisplayEmails() // Refresh current page
		}
	})
}

// OPTIMIZATION: Clear all emails with confirmation for large datasets
func (et *EmailsTab) ClearAllEmails() {
	if len(et.emails) == 0 {
		return
	}

	message := fmt.Sprintf("Remove all %s emails?", et.formatNumber(len(et.emails)))
	if len(et.emails) > 100000 {
		message += "\n\nThis is a large dataset and may take a moment to clear."
	}

	dialog.ShowConfirm("Clear All Emails", message,
		func(confirmed bool) {
			if confirmed {
				// Show progress for large datasets
				if len(et.emails) > 50000 {
					progress := dialog.NewProgressInfinite("Clearing", "Clearing all emails...", et.gui.window)
					progress.Show()

					// Clear in background for large datasets
					go func() {
						defer progress.Hide()

						// Clear cached stats
						et.lastStats = make(map[string]int)

						// Clear both emails and emailData, then sync
						et.emails = []string{}
						et.totalEmailCount = 0
						et.currentPage = 0
						et.displayEmails = []string{}

						et.gui.updateUI <- func() {
							et.emailData = binding.NewStringList()
							et.emailsList.Refresh()
							et.clearEmailStatusCache()
							et.updateStats()
							et.updatePageInfo()
							et.gui.updateStatus("Cleared all emails")
							et.addLog("🗑️ Đã xóa hết emails")
						}
					}()
				} else {
					// Immediate clear for small datasets
					et.lastStats = make(map[string]int)
					et.emails = []string{}
					et.totalEmailCount = 0
					et.currentPage = 0
					et.displayEmails = []string{}
					et.emailData = binding.NewStringList()
					et.emailsList.Refresh()
					et.clearEmailStatusCache()
					et.updateStats()
					et.updatePageInfo()
					et.gui.updateStatus("Cleared all emails")
					et.addLog("🗑️ Đã xóa hết emails")
				}
			}
		}, et.gui.window)
}

// OPTIMIZATION: Save emails with chunked processing for large datasets
func (et *EmailsTab) SaveEmails() {
	if len(et.emails) == 0 {
		return
	}

	// Show progress for large datasets
	if len(et.emails) > 50000 {
		progress := dialog.NewProgressInfinite("Saving", "Saving emails to file...", et.gui.window)
		progress.Show()

		go func() {
			defer progress.Hide()
			et.saveEmailsToFile()
		}()
	} else {
		et.saveEmailsToFile()
	}
}

func (et *EmailsTab) saveEmailsToFile() {
	var lines []string
	lines = append(lines, "# Target email addresses")
	lines = append(lines, fmt.Sprintf("# Total emails: %s", et.formatNumber(len(et.emails))))
	lines = append(lines, fmt.Sprintf("# Generated: %s", time.Now().Format("2006-01-02 15:04:05")))
	lines = append(lines, "")

	// Remove duplicates before saving
	uniqueEmails := utils.RemoveDuplicateEmails(et.emails)

	for _, email := range uniqueEmails {
		lines = append(lines, email)
	}

	content := strings.Join(lines, "\n")
	err := os.WriteFile("emails.txt", []byte(content), 0644)
	if err != nil {
		et.gui.updateUI <- func() {
			et.gui.updateStatus(fmt.Sprintf("Failed to save: %v", err))
		}
		return
	}

	// Update internal emails list if duplicates were removed
	duplicatesRemoved := len(et.emails) - len(uniqueEmails)
	if duplicatesRemoved > 0 {
		et.emails = uniqueEmails
		et.totalEmailCount = len(uniqueEmails)
		et.updateDisplayEmails()
	}

	et.gui.updateUI <- func() {
		et.gui.updateStatus(fmt.Sprintf("Saved %s emails", et.formatNumber(len(uniqueEmails))))
		if duplicatesRemoved > 0 {
			et.addLog(fmt.Sprintf("💾 Saved %s emails to file (removed %s duplicates)",
				et.formatNumber(len(uniqueEmails)), et.formatNumber(duplicatesRemoved)))
		} else {
			et.addLog(fmt.Sprintf("💾 Saved %s emails to file", et.formatNumber(len(uniqueEmails))))
		}
	}
}

// OPTIMIZATION: Load emails with streaming for large files
func (et *EmailsTab) LoadEmails() {
	emailStorage := storageInternal.NewEmailStorage()

	// Check file size first
	if fileInfo, err := os.Stat("emails.txt"); err == nil {
		fileSize := fileInfo.Size()
		if fileSize > 10*1024*1024 { // > 10MB
			// Show progress for large files
			progress := dialog.NewProgressInfinite("Loading", "Loading large email file...", et.gui.window)
			progress.Show()

			go func() {
				defer progress.Hide()
				et.loadEmailsFromStorage(emailStorage)
			}()
			return
		}
	}

	// Direct load for small files
	et.loadEmailsFromStorage(emailStorage)
}

func (et *EmailsTab) loadEmailsFromStorage(emailStorage *storageInternal.EmailStorage) {
	emails, err := emailStorage.LoadEmailsFromFile("emails.txt")
	if err != nil {
		if _, err := os.Stat("emails.txt"); os.IsNotExist(err) {
			sampleContent := `# Target email addresses
example@example.com
`
			os.WriteFile("emails.txt", []byte(sampleContent), 0644)
		}
		et.gui.updateUI <- func() {
			et.gui.updateStatus("No emails file found")
		}
		return
	}

	// Store all emails
	et.emails = emails
	et.totalEmailCount = len(emails)
	et.currentPage = 0

	// Update display with pagination
	et.gui.updateUI <- func() {
		et.updateDisplayEmails()
		et.clearEmailStatusCache()
		et.updateStats()
		et.gui.updateStatus(fmt.Sprintf("Loaded %s emails (showing page 1/%d)",
			et.formatNumber(len(emails)), et.getTotalPages()))
		et.addLog(fmt.Sprintf("📂 Loaded %s emails from file", et.formatNumber(len(emails))))
	}
}

func (et *EmailsTab) RefreshEmailsList() {
	et.LoadEmails()
	// Also update stats from database when refreshing
	et.updateStatsFromDatabase()
}

// OPTIMIZATION: Throttled stats update
func (et *EmailsTab) updateStats() {
	total := et.totalEmailCount

	// If crawler is running, get real stats
	if et.autoCrawler != nil {
		emailStorage, _, _ := et.autoCrawler.GetStorageServices()
		if emailStorage != nil {
			if stats, err := emailStorage.GetEmailStats(); err == nil {
				pending := stats["pending"]
				success := stats["success"]
				failed := stats["failed"]
				hasInfo := stats["has_info"]
				noInfo := stats["no_info"]

				et.totalLabel.SetText(fmt.Sprintf("Total: %s", et.formatNumber(total)))
				et.pendingLabel.SetText(fmt.Sprintf("Pending: %s", et.formatNumber(pending)))
				et.successLabel.SetText(fmt.Sprintf("Success: %s", et.formatNumber(success)))
				et.failedLabel.SetText(fmt.Sprintf("Failed: %s", et.formatNumber(failed)))
				et.hasInfoLabel.SetText(fmt.Sprintf("Has LinkedIn: %s", et.formatNumber(hasInfo)))
				et.noInfoLabel.SetText(fmt.Sprintf("No LinkedIn: %s", et.formatNumber(noInfo)))

				// Cache stats
				et.lastStats = stats
				return
			}
		}
	}

	// Try to get stats from database when not crawling with logging
	et.updateStatsFromDatabase()
}

// Update stats with formatted numbers
func (et *EmailsTab) updateStatsFromDatabase() {
	// Nếu đang crawling, dùng stats từ crawler
	if atomic.LoadInt32(&et.isCrawling) == 1 {
		et.updateStatsFromCrawler()
		return
	}

	// Nếu có cached stats và không crawling, dùng cached stats
	if len(et.lastStats) > 0 {
		et.updateStatsFromCache()
		return
	}

	// Try to get stats from database directly
	emailStorage := storageInternal.NewEmailStorage()

	// Initialize database connection
	if err := emailStorage.InitDB(); err != nil {
		et.updateStatsDefault()
		return
	}
	defer emailStorage.CloseDB()

	stats, err := emailStorage.GetEmailStats()
	if err != nil {
		// Fallback to cached stats or default
		if len(et.lastStats) > 0 {
			et.updateStatsFromCache()
		} else {
			et.updateStatsDefault()
		}
		return
	}

	total := et.totalEmailCount
	pending := stats["pending"]
	success := stats["success"]
	failed := stats["failed"]
	hasInfo := stats["has_info"]
	noInfo := stats["no_info"]

	et.totalLabel.SetText(fmt.Sprintf("Total: %s", et.formatNumber(total)))
	et.pendingLabel.SetText(fmt.Sprintf("Pending: %s", et.formatNumber(pending)))
	et.successLabel.SetText(fmt.Sprintf("Success: %s", et.formatNumber(success)))
	et.failedLabel.SetText(fmt.Sprintf("Failed: %s", et.formatNumber(failed)))
	et.hasInfoLabel.SetText(fmt.Sprintf("Has LinkedIn: %s", et.formatNumber(hasInfo)))
	et.noInfoLabel.SetText(fmt.Sprintf("No LinkedIn: %s", et.formatNumber(noInfo)))

	// Cache stats
	et.lastStats = stats
}

func (et *EmailsTab) updateStatsFromCache() {
	if len(et.lastStats) == 0 {
		et.updateStatsDefault()
		return
	}

	total := et.totalEmailCount
	pending := et.lastStats["pending"]
	success := et.lastStats["success"]
	failed := et.lastStats["failed"]
	hasInfo := et.lastStats["has_info"]
	noInfo := et.lastStats["no_info"]

	et.totalLabel.SetText(fmt.Sprintf("Total: %s", et.formatNumber(total)))
	et.pendingLabel.SetText(fmt.Sprintf("Pending: %s", et.formatNumber(pending)))
	et.successLabel.SetText(fmt.Sprintf("Success: %s", et.formatNumber(success)))
	et.failedLabel.SetText(fmt.Sprintf("Failed: %s", et.formatNumber(failed)))
	et.hasInfoLabel.SetText(fmt.Sprintf("Has LinkedIn: %s", et.formatNumber(hasInfo)))
	et.noInfoLabel.SetText(fmt.Sprintf("No LinkedIn: %s", et.formatNumber(noInfo)))
}

func (et *EmailsTab) updateStatsFromCrawler() {
	if et.autoCrawler == nil {
		return
	}

	// Get stats from crawler's storage
	emailStorage, _, _ := et.autoCrawler.GetStorageServices()
	if emailStorage != nil {
		stats, err := emailStorage.GetEmailStats()
		if err == nil {
			total := et.totalEmailCount
			pending := stats["pending"]
			success := stats["success"]
			failed := stats["failed"]
			hasInfo := stats["has_info"]
			noInfo := stats["no_info"]

			et.totalLabel.SetText(fmt.Sprintf("Total: %s", et.formatNumber(total)))
			et.pendingLabel.SetText(fmt.Sprintf("Pending: %s", et.formatNumber(pending)))
			et.successLabel.SetText(fmt.Sprintf("Success: %s", et.formatNumber(success)))
			et.failedLabel.SetText(fmt.Sprintf("Failed: %s", et.formatNumber(failed)))
			et.hasInfoLabel.SetText(fmt.Sprintf("Has LinkedIn: %s", et.formatNumber(hasInfo)))
			et.noInfoLabel.SetText(fmt.Sprintf("No LinkedIn: %s", et.formatNumber(noInfo)))

			// Update progress bar
			if total > 0 {
				processed := success + failed
				progress := float64(processed) / float64(total)
				if et.progressBar != nil {
					et.progressBar.SetValue(progress)
				}
				if et.progressLabel != nil {
					et.progressLabel.SetText(fmt.Sprintf("Progress: %s/%s (%.1f%%)",
						et.formatNumber(processed), et.formatNumber(total), progress*100))
				}
			}

			// Cache stats
			et.lastStats = stats

			// Log progress periodically for large datasets
			processed := success + failed
			if processed > 0 && processed%1000 == 0 { // Log every 1000 processed
				progressPercent := float64(processed) * 100 / float64(total)
				et.addLog(fmt.Sprintf("📊 Progress: %.1f%% (%s/%s) | Success: %s | Failed: %s | LinkedIn: %s",
					progressPercent, et.formatNumber(processed), et.formatNumber(total),
					et.formatNumber(success), et.formatNumber(failed), et.formatNumber(hasInfo)))
			}
		}
	}
}

func (et *EmailsTab) updateStatsDefault() {
	// Nếu có cached stats, dùng cached stats thay vì reset về 0
	if len(et.lastStats) > 0 {
		et.updateStatsFromCache()
		return
	}

	total := et.totalEmailCount
	et.totalLabel.SetText(fmt.Sprintf("Total: %s", et.formatNumber(total)))
	et.pendingLabel.SetText(fmt.Sprintf("Pending: %s", et.formatNumber(total)))
	et.successLabel.SetText(fmt.Sprintf("Success: %s", et.formatNumber(0)))
	et.failedLabel.SetText(fmt.Sprintf("Failed: %s", et.formatNumber(0)))
	et.hasInfoLabel.SetText(fmt.Sprintf("Has LinkedIn: %s", et.formatNumber(0)))
	et.noInfoLabel.SetText(fmt.Sprintf("No LinkedIn: %s", et.formatNumber(0)))
}

// finalizeAfterStop - Xử lý sau khi stop crawling
func (et *EmailsTab) finalizeAfterStop() {
	if et.autoCrawler != nil {
		// Get final stats from autoCrawler
		emailStorage, _, _ := et.autoCrawler.GetStorageServices()
		config := et.autoCrawler.GetConfig()
		if emailStorage != nil {
			// Export pending emails back to emails.txt
			err := emailStorage.ExportPendingEmailsToFile(config.EmailsFilePath)
			if err != nil {
				et.addLog(fmt.Sprintf("⚠️ Không thể export pending emails: %v", err))
			} else {
				// Get pending count for log
				pendingEmails, err := emailStorage.GetPendingEmails()
				if err == nil {
					if len(pendingEmails) > 0 {
						et.addLog(fmt.Sprintf("💾 Đã lưu %s emails pending vào file emails.txt", et.formatNumber(len(pendingEmails))))
					} else {
						et.addLog("✅ Tất cả emails đã được xử lý xong!")
					}
				}
			}

			// Get final stats và lưu vào cache
			stats, err := emailStorage.GetEmailStats()
			if err == nil {
				et.lastStats = stats // Cache stats để tránh reset về 0
				et.addLog(fmt.Sprintf("📊 Trạng thái cuối: Success: %s | Failed: %s | LinkedIn: %s",
					et.formatNumber(stats["success"]), et.formatNumber(stats["failed"]), et.formatNumber(stats["has_info"])))
			}

			// Close database properly
			emailStorage.CloseDB()
		}
	}
}

func (et *EmailsTab) clearEmailStatusCache() {
	et.emailStatusCache = make(map[string]string)
	et.lastCacheUpdate = time.Time{}
}

func (et *EmailsTab) updateEmailStatusCache() {
	// Only update cache every 5 seconds to avoid excessive database queries
	if time.Since(et.lastCacheUpdate) < 5*time.Second {
		return
	}

	emailStorage := storageInternal.NewEmailStorage()
	if err := emailStorage.InitDB(); err != nil {
		return
	}
	defer emailStorage.CloseDB()

	// OPTIMIZATION: Only cache status for visible emails to save memory
	query := `SELECT email, status, has_info, no_info FROM emails LIMIT 1000`
	db := emailStorage.GetDB()
	if db == nil {
		return
	}

	rows, err := db.Query(query)
	if err != nil {
		return
	}
	defer rows.Close()

	newCache := make(map[string]string)
	for rows.Next() {
		var email, status string
		var hasInfo, noInfo bool

		if err := rows.Scan(&email, &status, &hasInfo, &noInfo); err != nil {
			continue
		}

		// Convert database status to display status
		switch status {
		case "pending":
			newCache[email] = "Pending"
		case "success":
			if hasInfo {
				newCache[email] = "Success - Has LinkedIn"
			} else {
				newCache[email] = "Success - No LinkedIn"
			}
		case "failed":
			newCache[email] = "Failed"
		default:
			newCache[email] = "Unknown"
		}
	}

	et.emailStatusCache = newCache
	et.lastCacheUpdate = time.Now()
}

func (et *EmailsTab) getEmailStatus(email string) string {
	// If we have crawler running, get live status
	if et.autoCrawler != nil {
		emailStorage, _, _ := et.autoCrawler.GetStorageServices()
		if emailStorage != nil {
			// Try to get status from running crawler's database
			if status, ok := et.emailStatusCache[email]; ok {
				return status
			}
			return "Processing"
		}
	}

	// Update cache if needed
	et.updateEmailStatusCache()

	// Return cached status if available
	if status, ok := et.emailStatusCache[email]; ok {
		return status
	}

	// Default to Pending if not found in cache
	return "Pending"
}

func (et *EmailsTab) checkTokensAvailability() bool {
	// First priority: Check if tokens.txt exists and has valid tokens
	if et.hasValidTokensFile() {
		et.addLog("✅ Tìm thấy file tokens.txt với tokens hợp lệ")
		return true
	}

	et.addLog("⚠️ Không tìm thấy tokens hợp lệ trong file tokens.txt")

	// Second priority: Check if accounts are available for token extraction
	accountsTab := et.gui.accountsTab
	if accountsTab != nil && len(accountsTab.GetAccounts()) > 0 {
		et.addLog("✅ Tìm thấy accounts để extract tokens")
		return true
	}

	et.addLog("❌ Không có accounts để extract tokens")
	return false
}

func (et *EmailsTab) hasValidTokensFile() bool {
	tokenStorage := storageInternal.NewTokenStorage()
	tokens, err := tokenStorage.LoadTokensFromFile("tokens.txt")
	if err != nil {
		et.addLog(fmt.Sprintf("🔍 Không thể đọc file tokens.txt: %v", err))
		return false
	}

	if len(tokens) == 0 {
		et.addLog("🔍 File tokens.txt rỗng hoặc không có tokens")
		return false
	}

	et.addLog(fmt.Sprintf("🔍 Tìm thấy %d tokens trong file tokens.txt", len(tokens)))

	// Use utils package for validation
	validCount, _ := utils.ValidateTokenBatch(tokens)

	if validCount == 0 {
		et.addLog("⚠️ Không có tokens nào có format hợp lệ trong file")
		return false
	}

	et.addLog(fmt.Sprintf("✅ Có %d/%d tokens có format hợp lệ", validCount, len(tokens)))
	return validCount > 0
}

func (et *EmailsTab) logTokenAccountStatus() {
	// Check tokens
	tokenStorage := storageInternal.NewTokenStorage()
	tokens, err := tokenStorage.LoadTokensFromFile("tokens.txt")
	if err == nil && len(tokens) > 0 {
		et.addLog(fmt.Sprintf("🔑 Tokens khả dụng: %d tokens từ file", len(tokens)))
	} else {
		et.addLog("🔑 Không có tokens trong file, sẽ extract từ accounts")
	}

	// Check accounts
	accountsTab := et.gui.accountsTab
	if accountsTab != nil {
		accounts := accountsTab.GetAccounts()
		if len(accounts) > 0 {
			et.addLog(fmt.Sprintf("👥 Accounts khả dụng: %d accounts để extract tokens", len(accounts)))
		} else {
			et.addLog("👥 Không có accounts để extract tokens")
		}
	}
}

func (et *EmailsTab) performEmailCrawling(ctx context.Context) {
	et.gui.updateUI <- func() {
		et.addLog("🔧 Đang khởi tạo crawler...")
	}

	// Create config
	cfg := config.DefaultConfig()
	cfg.EmailsFilePath = "emails.txt"
	cfg.TokensFilePath = "tokens.txt"
	cfg.AccountsFilePath = "accounts.txt"
	cfg.MaxConcurrency = 20
	cfg.RequestsPerSec = 15.0

	// Initialize AutoCrawler
	autoCrawler, err := orchestrator.New(cfg)
	if err != nil {
		et.gui.updateUI <- func() {
			et.addLog(fmt.Sprintf("❌ Lỗi khởi tạo crawler: %v", err))
		}
		return
	}

	et.autoCrawler = autoCrawler
	et.gui.updateUI <- func() {
		et.addLog("✅ Crawler đã sẵn sàng!")
		et.addLog("🔄 Bắt đầu quá trình crawling...")
	}

	// Start progress monitoring
	go et.monitorCrawlProgress(ctx)

	// Run the crawler
	err = autoCrawler.Run()

	if err != nil {
		et.gui.updateUI <- func() {
			et.addLog(fmt.Sprintf("⚠️ Crawler kết thúc với lỗi: %v", err))
		}
	} else {
		et.gui.updateUI <- func() {
			et.addLog("🎉 Crawler hoàn thành thành công!")
		}
	}

	// Show final results
	et.gui.updateUI <- func() {
		et.showFinalResults()
		// Clear cache and update stats from database after completion
		et.clearEmailStatusCache()
		et.updateStatsFromDatabase()
		// Refresh current page
		et.updateDisplayEmails()
	}
}

func (et *EmailsTab) Cleanup() {
	// Stop stats refresh ticker
	if et.statsRefreshTicker != nil {
		et.statsRefreshTicker.Stop()
		et.statsRefreshTicker = nil
	}

	// Clear cache
	et.emailStatusCache = nil

	// Clear log buffer to free memory
	et.logBuffer = nil

	// Close any database connections
	if et.autoCrawler != nil {
		emailStorage, _, _ := et.autoCrawler.GetStorageServices()
		if emailStorage != nil {
			emailStorage.CloseDB()
		}
	}
}

func (et *EmailsTab) monitorCrawlProgress(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second) // Slower updates for large datasets
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if et.autoCrawler != nil {
				et.gui.updateUI <- func() {
					et.updateStatsFromCrawler()
					// Clear cache periodically during crawling to get fresh data
					et.clearEmailStatusCache()
				}
			}
		}
	}
}

func (et *EmailsTab) showFinalResults() {
	if et.autoCrawler == nil {
		return
	}

	emailStorage, _, _ := et.autoCrawler.GetStorageServices()
	if emailStorage != nil {
		stats, err := emailStorage.GetEmailStats()
		if err == nil {
			total := et.totalEmailCount
			success := stats["success"]
			failed := stats["failed"]
			hasInfo := stats["has_info"]
			noInfo := stats["no_info"]

			et.addLog("🎉 KẾT QUẢ CUỐI CÙNG:")
			et.addLog(fmt.Sprintf("📊 Tổng emails: %s", et.formatNumber(total)))
			et.addLog(fmt.Sprintf("✅ Thành công: %s", et.formatNumber(success)))
			et.addLog(fmt.Sprintf("❌ Thất bại: %s", et.formatNumber(failed)))
			et.addLog(fmt.Sprintf("🎯 Có LinkedIn: %s", et.formatNumber(hasInfo)))
			et.addLog(fmt.Sprintf("📭 Không có LinkedIn: %s", et.formatNumber(noInfo)))

			if hasInfo > 0 {
				et.addLog(fmt.Sprintf("🎉 Tìm thấy %s LinkedIn profiles - Xem trong file hit.txt!", et.formatNumber(hasInfo)))
			}

			successRate := 0.0
			if total > 0 {
				successRate = float64(success) * 100 / float64(total)
			}
			et.addLog(fmt.Sprintf("📈 Tỷ lệ thành công: %.1f%%", successRate))

			// Cache final stats
			et.lastStats = stats
		}
	}

	// Refresh results tab
	if et.gui.resultsTab != nil {
		et.gui.resultsTab.RefreshResults()
	}
}

func (et *EmailsTab) addLog(msg string) {
	ts := time.Now().Format("15:04:05")
	logEntry := fmt.Sprintf("[%s] %s", ts, msg)
	et.logBuffer = append(et.logBuffer, logEntry)

	// Keep only last 200 entries
	if len(et.logBuffer) > 200 {
		et.logBuffer = et.logBuffer[len(et.logBuffer)-200:]
	}

	// Update display
	displayText := "```\n" + strings.Join(et.logBuffer, "\n") + "\n```"
	et.logText.ParseMarkdown(displayText)
}

func (et *EmailsTab) GetEmails() []string {
	return et.emails
}

func (et *EmailsTab) OnCrawlerStarted() {
	if et.statusLabel != nil {
		et.statusLabel.SetText("Status: Running")
	}
	if et.progressLabel != nil {
		et.progressLabel.SetText("Initializing crawler...")
	}
	if et.progressBar != nil {
		et.progressBar.SetValue(0)
	}
	et.addLog("🚀 Crawler started!")
}

func (et *EmailsTab) OnCrawlerStopped() {
	if et.statusLabel != nil {
		et.statusLabel.SetText("Status: Stopped")
	}
	if et.progressLabel != nil {
		et.progressLabel.SetText("Done.")
	}
	if et.progressBar != nil {
		et.progressBar.SetValue(1.0)
	}
	et.addLog("⏹️ Crawler stopped.")
}

func (et *EmailsTab) addCrawlerLog(msg string) {
	timestamp := time.Now().Format("15:04:05")
	line := fmt.Sprintf("[%s] %s", timestamp, msg)
	et.logBuffer = append(et.logBuffer, line)
	if et.logText != nil {
		et.logText.ParseMarkdown(strings.Join(et.logBuffer, "\n"))
	}
}
