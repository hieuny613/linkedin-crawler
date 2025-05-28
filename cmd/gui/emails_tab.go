package main

import (
	"context"
	"fmt"
	"io"
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

	totalLabel   *widget.Label
	pendingLabel *widget.Label
	successLabel *widget.Label
	failedLabel  *widget.Label
	hasInfoLabel *widget.Label
	noInfoLabel  *widget.Label

	selectedIndex int

	// Crawling state
	isCrawling  int32 // atomic flag
	crawlCancel context.CancelFunc
	autoCrawler *orchestrator.AutoCrawler
}

func NewEmailsTab(gui *CrawlerGUI) *EmailsTab {
	tab := &EmailsTab{
		gui:       gui,
		emails:    []string{},
		emailData: binding.NewStringList(),
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

	tab.setupEmailsList()
	return tab
}

func (et *EmailsTab) CreateContent() fyne.CanvasObject {
	fileButtons := container.NewHBox(
		et.importBtn,
		et.clearBtn,
		widget.NewButton("Refresh", et.RefreshEmailsList),
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
		container.NewScroll(et.emailsList),
	)

	// Control buttons
	controlButtons := container.NewVBox(
		et.startCrawlBtn,
		et.stopCrawlBtn,
	)

	// Log area - MỞ RỘNG XUỐNG DƯỚI
	logScroll := container.NewScroll(et.logText)
	logArea := container.NewBorder(
		widget.NewLabel("Email Crawl Log:"), nil, nil, nil,
		logScroll,
	)

	// Right panel with expanded log area
	rightPanel := container.NewBorder(
		widget.NewCard("Email Crawl Control", "", controlButtons),
		nil, nil, nil,
		widget.NewCard("Logs", "", logArea), // Log area chiếm phần lớn không gian
	)

	content := container.NewHSplit(leftPanel, rightPanel)
	content.SetOffset(0.5) // 50-50 split
	return content
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
			status := et.getEmailStatus(str)
			statusLabel.SetText(status)
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
		if id < len(et.emails) {
			et.selectedIndex = int(id)
		}
	}
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

	et.addLog("🚀 Bắt đầu crawl emails...")
	et.addLog(fmt.Sprintf("📊 Tổng số emails: %d", len(et.emails)))

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
				et.updateStats() // Update final stats
			}
		}()

		et.performEmailCrawling(ctx)
	}()
}

// STOP CRAWL - Hoạt động thực tế
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
}

// checkTokensAvailability checks if tokens are available, fallback to accounts
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

// hasValidTokensFile checks if tokens.txt exists and contains valid tokens
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

	// Quick validation - check if tokens look valid (basic format check)
	validCount := 0
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		// Basic token format validation (should be long alphanumeric string)
		if len(token) > 50 && et.isAlphanumericToken(token) {
			validCount++
		}
	}

	if validCount == 0 {
		et.addLog("⚠️ Không có tokens nào có format hợp lệ trong file")
		return false
	}

	et.addLog(fmt.Sprintf("✅ Có %d/%d tokens có format hợp lệ", validCount, len(tokens)))
	return validCount > 0
}

// isAlphanumericToken checks if token has valid format
func (et *EmailsTab) isAlphanumericToken(token string) bool {
	// Basic check for token format - should contain alphanumeric and some special chars
	// LinkedIn tokens typically contain letters, numbers, dots, underscores, and hyphens
	matched, _ := regexp.MatchString(`^[A-Za-z0-9._-]+$`, token)
	return matched
}

// logTokenAccountStatus logs the current token and account status
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

// performEmailCrawling thực hiện việc crawl emails
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
	}
}

// monitorCrawlProgress monitors crawling progress
func (et *EmailsTab) monitorCrawlProgress(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if et.autoCrawler != nil {
				et.gui.updateUI <- func() {
					et.updateStatsFromCrawler()
				}
			}
		}
	}
}

// updateStatsFromCrawler updates stats from running crawler
func (et *EmailsTab) updateStatsFromCrawler() {
	if et.autoCrawler == nil {
		return
	}

	// Get stats from crawler's storage
	emailStorage, _, _ := et.autoCrawler.GetStorageServices()
	if emailStorage != nil {
		stats, err := emailStorage.GetEmailStats()
		if err == nil {
			total := len(et.emails)
			pending := stats["pending"]
			success := stats["success"]
			failed := stats["failed"]
			hasInfo := stats["has_info"]
			noInfo := stats["no_info"]

			et.totalLabel.SetText(fmt.Sprintf("Total: %d", total))
			et.pendingLabel.SetText(fmt.Sprintf("Pending: %d", pending))
			et.successLabel.SetText(fmt.Sprintf("Success: %d", success))
			et.failedLabel.SetText(fmt.Sprintf("Failed: %d", failed))
			et.hasInfoLabel.SetText(fmt.Sprintf("Has LinkedIn: %d", hasInfo))
			et.noInfoLabel.SetText(fmt.Sprintf("No LinkedIn: %d", noInfo))

			// Log progress periodically
			processed := success + failed
			if processed > 0 && processed%10 == 0 {
				progress := float64(processed) * 100 / float64(total)
				et.addLog(fmt.Sprintf("📊 Tiến độ: %.1f%% (%d/%d) | Success: %d | Failed: %d | LinkedIn: %d",
					progress, processed, total, success, failed, hasInfo))
			}
		}
	}
}

// showFinalResults shows final crawling results
func (et *EmailsTab) showFinalResults() {
	if et.autoCrawler == nil {
		return
	}

	emailStorage, _, _ := et.autoCrawler.GetStorageServices()
	if emailStorage != nil {
		stats, err := emailStorage.GetEmailStats()
		if err == nil {
			total := len(et.emails)
			success := stats["success"]
			failed := stats["failed"]
			hasInfo := stats["has_info"]
			noInfo := stats["no_info"]

			et.addLog("🎉 KẾT QUẢ CUỐI CÙNG:")
			et.addLog(fmt.Sprintf("📊 Tổng emails: %d", total))
			et.addLog(fmt.Sprintf("✅ Thành công: %d", success))
			et.addLog(fmt.Sprintf("❌ Thất bại: %d", failed))
			et.addLog(fmt.Sprintf("🎯 Có LinkedIn: %d", hasInfo))
			et.addLog(fmt.Sprintf("📭 Không có LinkedIn: %d", noInfo))

			if hasInfo > 0 {
				et.addLog(fmt.Sprintf("🎉 Tìm thấy %d LinkedIn profiles - Xem trong file hit.txt!", hasInfo))
			}

			successRate := 0.0
			if total > 0 {
				successRate = float64(success) * 100 / float64(total)
			}
			et.addLog(fmt.Sprintf("📈 Tỷ lệ thành công: %.1f%%", successRate))
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

func (et *EmailsTab) ImportEmails() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()
		content, err := io.ReadAll(reader)
		if err != nil {
			et.gui.updateUI <- func() {
				dialog.ShowError(fmt.Errorf("Failed to read file: %v", err), et.gui.window)
			}
			return
		}
		if len(content) == 0 {
			et.gui.updateUI <- func() {
				dialog.ShowInformation("Empty File", "The selected file is empty", et.gui.window)
			}
			return
		}
		lines := strings.Split(string(content), "\n")
		imported := 0
		skipped := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			email := line
			if strings.Contains(line, ",") {
				parts := strings.Split(line, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if et.isValidEmail(part) {
						email = part
						break
					}
				}
			}
			if !et.isValidEmail(email) {
				continue
			}
			exists := false
			for _, existingEmail := range et.emails {
				if existingEmail == email {
					exists = true
					skipped++
					break
				}
			}
			if !exists {
				et.emails = append(et.emails, email)
				et.emailData.Append(email)
				imported++
			}
		}
		et.gui.updateUI <- func() {
			et.updateStats()
			message := fmt.Sprintf("Imported: %d | Skipped: %d", imported, skipped)
			dialog.ShowInformation("Import Results", message, et.gui.window)
			et.gui.updateStatus(fmt.Sprintf("Imported %d emails", imported))
			et.addLog(fmt.Sprintf("📥 Import: %d emails thành công, %d bị bỏ qua", imported, skipped))
		}
	}, et.gui.window)
}

func (et *EmailsTab) ClearAllEmails() {
	if len(et.emails) == 0 {
		return
	}
	dialog.ShowConfirm("Clear All Emails",
		fmt.Sprintf("Remove all %d emails?", len(et.emails)),
		func(confirmed bool) {
			if confirmed {
				et.emails = []string{}
				et.emailData = binding.NewStringList()
				et.setupEmailsList()
				et.updateStats()
				et.gui.updateUI <- func() {
					et.gui.updateStatus("Cleared all emails")
					et.addLog("🗑️ Đã xóa hết emails")
				}
			}
		}, et.gui.window)
}

func (et *EmailsTab) LoadEmails() {
	emailStorage := storageInternal.NewEmailStorage()
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
	et.emails = []string{}
	et.emailData = binding.NewStringList()
	et.setupEmailsList()
	for _, email := range emails {
		et.emails = append(et.emails, email)
		et.emailData.Append(email)
	}
	et.updateStats()
	et.gui.updateUI <- func() {
		et.gui.updateStatus(fmt.Sprintf("Loaded %d emails", len(emails)))
		et.addLog(fmt.Sprintf("📂 Loaded %d emails từ file", len(emails)))
	}
}

func (et *EmailsTab) SaveEmails() {
	if len(et.emails) == 0 {
		return
	}
	var lines []string
	lines = append(lines, "# Target email addresses")
	lines = append(lines, "")
	for _, email := range et.emails {
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
	et.gui.updateUI <- func() {
		et.gui.updateStatus(fmt.Sprintf("Saved %d emails", len(et.emails)))
		et.addLog(fmt.Sprintf("💾 Saved %d emails to file", len(et.emails)))
	}
}

func (et *EmailsTab) RefreshEmailsList() {
	et.LoadEmails()
}

func (et *EmailsTab) isValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

func (et *EmailsTab) getEmailStatus(email string) string {
	if et.autoCrawler != nil {
		emailStorage, _, _ := et.autoCrawler.GetStorageServices()
		if emailStorage != nil {
			// Try to get status from database
			return "Processing"
		}
	}
	return "Pending"
}

func (et *EmailsTab) updateStats() {
	total := len(et.emails)

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

				et.totalLabel.SetText(fmt.Sprintf("Total: %d", total))
				et.pendingLabel.SetText(fmt.Sprintf("Pending: %d", pending))
				et.successLabel.SetText(fmt.Sprintf("Success: %d", success))
				et.failedLabel.SetText(fmt.Sprintf("Failed: %d", failed))
				et.hasInfoLabel.SetText(fmt.Sprintf("Has LinkedIn: %d", hasInfo))
				et.noInfoLabel.SetText(fmt.Sprintf("No LinkedIn: %d", noInfo))
				return
			}
		}
	}

	// Default stats when not crawling
	et.totalLabel.SetText(fmt.Sprintf("Total: %d", total))
	et.pendingLabel.SetText(fmt.Sprintf("Pending: %d", total))
	et.successLabel.SetText(fmt.Sprintf("Success: %d", 0))
	et.failedLabel.SetText(fmt.Sprintf("Failed: %d", 0))
	et.hasInfoLabel.SetText(fmt.Sprintf("Has LinkedIn: %d", 0))
	et.noInfoLabel.SetText(fmt.Sprintf("No LinkedIn: %d", 0))
}

func (et *EmailsTab) GetEmails() []string {
	return et.emails
}
