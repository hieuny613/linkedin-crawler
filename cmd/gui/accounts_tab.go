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

	"linkedin-crawler/internal/auth"
	"linkedin-crawler/internal/models"
	storageInternal "linkedin-crawler/internal/storage"
)

type AccountsTab struct {
	gui          *CrawlerGUI
	accountsList *widget.List
	accounts     []models.Account
	accountData  binding.StringList

	importBtn     *widget.Button
	cleanBtn      *widget.Button
	startTokenBtn *widget.Button
	stopTokenBtn  *widget.Button

	totalLabel     *widget.Label
	usedLabel      *widget.Label
	remainingLabel *widget.Label

	// Token info labels replacing quick actions
	validTokensLabel   *widget.Label
	invalidTokensLabel *widget.Label
	totalTokensLabel   *widget.Label
	lastUpdateLabel    *widget.Label

	logText   *widget.RichText
	logBuffer []string

	selectedIndex int

	// Token extraction state
	isTokenExtracting  int32 // atomic flag
	tokenExtractCancel context.CancelFunc
	tokenExtractor     *auth.TokenExtractor

	// Token info refresh ticker
	tokenInfoTicker *time.Ticker
}

func NewAccountsTab(gui *CrawlerGUI) *AccountsTab {
	tab := &AccountsTab{
		gui:            gui,
		accounts:       []models.Account{},
		accountData:    binding.NewStringList(),
		tokenExtractor: auth.NewTokenExtractor(),
	}

	tab.importBtn = widget.NewButtonWithIcon("Import", theme.FolderOpenIcon(), tab.ImportAccounts)
	tab.cleanBtn = widget.NewButtonWithIcon("Clean All", theme.DeleteIcon(), tab.CleanAllAccounts)
	tab.cleanBtn.Importance = widget.DangerImportance

	tab.startTokenBtn = widget.NewButtonWithIcon("Start Token Extract", theme.MediaPlayIcon(), tab.StartTokenExtract)
	tab.stopTokenBtn = widget.NewButtonWithIcon("Stop Token Extract", theme.MediaStopIcon(), tab.StopTokenExtract)
	tab.stopTokenBtn.Importance = widget.DangerImportance
	tab.stopTokenBtn.Disable() // Initially disabled

	tab.logText = widget.NewRichText()
	tab.logText.Wrapping = fyne.TextWrapWord
	tab.logBuffer = []string{}

	tab.totalLabel = widget.NewLabel("Total: 0")
	tab.usedLabel = widget.NewLabel("Used: 0")
	tab.remainingLabel = widget.NewLabel("Available: 0")

	// Initialize token info labels
	tab.validTokensLabel = widget.NewLabel("Valid Tokens: 0")
	tab.invalidTokensLabel = widget.NewLabel("Invalid Tokens: 0")
	tab.totalTokensLabel = widget.NewLabel("Total Tokens: 0")
	tab.lastUpdateLabel = widget.NewLabel("Last Update: Never")

	tab.setupAccountsList()

	// Start token info refresh ticker
	tab.startTokenInfoRefresh()

	return tab
}

func (at *AccountsTab) CreateContent() fyne.CanvasObject {
	fileButtons := container.NewHBox(
		at.importBtn,
		at.cleanBtn,
		widget.NewButton("Refresh", at.RefreshAccountsList),
	)

	statsGrid := container.NewHBox(
		at.totalLabel,
		widget.NewSeparator(),
		at.usedLabel,
		widget.NewSeparator(),
		at.remainingLabel,
	)

	// Token info grid replacing quick actions
	tokenInfoGrid := container.NewVBox(
		container.NewHBox(
			at.validTokensLabel,
			widget.NewSeparator(),
			at.invalidTokensLabel,
		),
		container.NewHBox(
			at.totalTokensLabel,
			widget.NewSeparator(),
			at.lastUpdateLabel,
		),
		widget.NewButton("Refresh Token Info", at.RefreshTokenInfo),
	)

	leftPanel := container.NewVBox(
		widget.NewCard("File Operations", "", fileButtons),
		widget.NewCard("Statistics", "", statsGrid),
		widget.NewCard("Token Information", "", tokenInfoGrid), // Replaced Quick Actions
		container.NewScroll(at.accountsList),
	)

	// Control buttons
	controlButtons := container.NewVBox(
		at.startTokenBtn,
		at.stopTokenBtn,
	)

	// Log area - MỞ RỘNG XUỐNG DƯỚI
	logScroll := container.NewScroll(at.logText)
	logArea := container.NewBorder(
		widget.NewLabel("Token Extraction Log:"), nil, nil, nil,
		logScroll,
	)

	// Right panel with expanded log area
	rightPanel := container.NewBorder(
		widget.NewCard("Token Control", "", controlButtons),
		nil, nil, nil,
		widget.NewCard("Logs", "", logArea), // Log area chiếm phần lớn không gian
	)

	content := container.NewHSplit(leftPanel, rightPanel)
	content.SetOffset(0.5) // 50-50 split
	return content
}

func (at *AccountsTab) setupAccountsList() {
	at.accountsList = widget.NewListWithData(
		at.accountData,
		func() fyne.CanvasObject {
			icon := widget.NewIcon(theme.AccountIcon())
			email := widget.NewLabel("Email")
			status := widget.NewLabel("Status")
			return container.NewHBox(icon, container.NewVBox(email, status))
		},
		func(id binding.DataItem, obj fyne.CanvasObject) {
			str, _ := id.(binding.String).Get()
			parts := strings.Split(str, "|")
			containerObj := obj.(*fyne.Container)
			icon := containerObj.Objects[0].(*widget.Icon)
			infoContainer := containerObj.Objects[1].(*fyne.Container)
			emailLabel := infoContainer.Objects[0].(*widget.Label)
			statusLabel := infoContainer.Objects[1].(*widget.Label)

			if len(parts) >= 2 {
				emailLabel.SetText(parts[0])
				status := at.getAccountStatus(parts[0])
				statusLabel.SetText(status)
				switch status {
				case "Ready":
					icon.SetResource(theme.ConfirmIcon())
				case "Used":
					icon.SetResource(theme.InfoIcon())
				case "Failed":
					icon.SetResource(theme.ErrorIcon())
				default:
					icon.SetResource(theme.AccountIcon())
				}
			}
		},
	)
	at.accountsList.OnSelected = func(id widget.ListItemID) {
		at.selectedIndex = int(id)
	}
	at.selectedIndex = -1
}

// Start token info refresh ticker
func (at *AccountsTab) startTokenInfoRefresh() {
	if at.tokenInfoTicker != nil {
		at.tokenInfoTicker.Stop()
	}

	at.tokenInfoTicker = time.NewTicker(10 * time.Second) // Update every 10 seconds
	go func() {
		// Initial update
		at.gui.updateUI <- func() {
			at.updateTokenInfo()
		}

		defer func() {
			if at.tokenInfoTicker != nil {
				at.tokenInfoTicker.Stop()
			}
		}()

		for {
			select {
			case <-at.tokenInfoTicker.C:
				at.gui.updateUI <- func() {
					at.updateTokenInfo()
				}
			case <-at.gui.ctx.Done():
				return
			}
		}
	}()
}

// Update token information from tokens.txt file
func (at *AccountsTab) updateTokenInfo() {
	tokenStorage := storageInternal.NewTokenStorage()
	tokens, err := tokenStorage.LoadTokensFromFile("tokens.txt")

	if err != nil {
		// No tokens file or error reading
		at.validTokensLabel.SetText("Valid Tokens: 0")
		at.invalidTokensLabel.SetText("Invalid Tokens: 0")
		at.totalTokensLabel.SetText("Total Tokens: 0")
		at.lastUpdateLabel.SetText("Last Update: No tokens file")
		return
	}

	totalTokens := len(tokens)
	validTokens := 0
	invalidTokens := 0

	// Basic validation - check token format
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if len(token) > 50 && at.isValidTokenFormat(token) {
			validTokens++
		} else {
			invalidTokens++
		}
	}

	// Update labels
	at.validTokensLabel.SetText(fmt.Sprintf("Valid Tokens: %d", validTokens))
	at.invalidTokensLabel.SetText(fmt.Sprintf("Invalid Tokens: %d", invalidTokens))
	at.totalTokensLabel.SetText(fmt.Sprintf("Total Tokens: %d", totalTokens))
	at.lastUpdateLabel.SetText(fmt.Sprintf("Last Update: %s", time.Now().Format("15:04:05")))

	// Change label colors based on token availability
	if validTokens > 10 {
		at.validTokensLabel.Importance = widget.SuccessImportance
	} else if validTokens > 5 {
		at.validTokensLabel.Importance = widget.WarningImportance
	} else if validTokens > 0 {
		at.validTokensLabel.Importance = widget.MediumImportance
	} else {
		at.validTokensLabel.Importance = widget.DangerImportance
	}

	if invalidTokens > 0 {
		at.invalidTokensLabel.Importance = widget.WarningImportance
	} else {
		at.invalidTokensLabel.Importance = widget.LowImportance
	}

	if totalTokens > 15 {
		at.totalTokensLabel.Importance = widget.SuccessImportance
	} else if totalTokens > 5 {
		at.totalTokensLabel.Importance = widget.MediumImportance
	} else {
		at.totalTokensLabel.Importance = widget.LowImportance
	}
}

// Manual refresh token info
func (at *AccountsTab) RefreshTokenInfo() {
	at.updateTokenInfo()
	at.addLog("🔄 Đã refresh thông tin tokens")
}

// Check if token has valid format
func (at *AccountsTab) isValidTokenFormat(token string) bool {
	// Basic check for token format - should contain alphanumeric and some special chars
	// LinkedIn tokens typically contain letters, numbers, dots, underscores, and hyphens
	matched, _ := regexp.MatchString(`^[A-Za-z0-9._-]+$`, token)
	return matched && len(token) > 50
}

// START TOKEN EXTRACT - Hoạt động thực tế
func (at *AccountsTab) StartTokenExtract() {
	// Check if already running
	if atomic.LoadInt32(&at.isTokenExtracting) == 1 {
		at.addLog("⚠️ Token extraction đã đang chạy!")
		return
	}

	// Check if there are accounts
	if len(at.accounts) == 0 {
		at.addLog("❌ Không có accounts để extract tokens!")
		dialog.ShowError(fmt.Errorf("Không có accounts để extract tokens"), at.gui.window)
		return
	}

	// Set running state
	atomic.StoreInt32(&at.isTokenExtracting, 1)
	at.startTokenBtn.Disable()
	at.stopTokenBtn.Enable()

	at.addLog("🚀 Bắt đầu extract tokens từ accounts...")
	at.addLog(fmt.Sprintf("📊 Tổng số accounts: %d", len(at.accounts)))

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	at.tokenExtractCancel = cancel

	// Run extraction in background
	go func() {
		defer func() {
			// Reset state when done
			atomic.StoreInt32(&at.isTokenExtracting, 0)
			at.gui.updateUI <- func() {
				at.startTokenBtn.Enable()
				at.stopTokenBtn.Disable()
				at.addLog("✅ Token extraction hoàn thành!")
				// Update token info after extraction
				at.updateTokenInfo()
			}
		}()

		at.performTokenExtraction(ctx)
	}()
}

// STOP TOKEN EXTRACT - Hoạt động thực tế
func (at *AccountsTab) StopTokenExtract() {
	if atomic.LoadInt32(&at.isTokenExtracting) == 0 {
		at.addLog("⚠️ Token extraction không đang chạy!")
		return
	}

	at.addLog("⏹️ Đang dừng token extraction...")

	// Cancel the context
	if at.tokenExtractCancel != nil {
		at.tokenExtractCancel()
	}

	// Reset state immediately
	atomic.StoreInt32(&at.isTokenExtracting, 0)
	at.startTokenBtn.Enable()
	at.stopTokenBtn.Disable()

	at.addLog("🛑 Đã dừng token extraction!")
	// Update token info after stopping
	at.updateTokenInfo()
}

// performTokenExtraction thực hiện việc extract tokens
func (at *AccountsTab) performTokenExtraction(ctx context.Context) {
	successCount := 0
	failCount := 0

	// Process accounts in batches of 3
	batchSize := 3
	for i := 0; i < len(at.accounts); i += batchSize {
		// Check if cancelled
		select {
		case <-ctx.Done():
			at.gui.updateUI <- func() {
				at.addLog("⚠️ Token extraction bị hủy bởi người dùng")
			}
			return
		default:
		}

		end := i + batchSize
		if end > len(at.accounts) {
			end = len(at.accounts)
		}

		batch := at.accounts[i:end]
		at.gui.updateUI <- func() {
			at.addLog(fmt.Sprintf("📦 Xử lý batch %d-%d (%d accounts)...", i+1, end, len(batch)))
		}

		// Extract tokens from batch
		results := at.tokenExtractor.ExtractTokensBatch(batch, "accounts.txt")

		var validTokens []string
		for _, result := range results {
			if result.Error != nil {
				failCount++
				at.gui.updateUI <- func() {
					at.addLog(fmt.Sprintf("❌ Lỗi account %s: %v", result.Account.Email, result.Error))
				}
			} else if result.Token != "" {
				successCount++
				validTokens = append(validTokens, result.Token)
				at.gui.updateUI <- func() {
					at.addLog(fmt.Sprintf("✅ Thành công account %s", result.Account.Email))
				}
			}
		}

		// Save tokens to file
		if len(validTokens) > 0 {
			tokenStorage := storageInternal.NewTokenStorage()
			err := tokenStorage.SaveTokensToFile("tokens.txt", validTokens)
			if err != nil {
				at.gui.updateUI <- func() {
					at.addLog(fmt.Sprintf("⚠️ Lỗi lưu tokens: %v", err))
				}
			} else {
				at.gui.updateUI <- func() {
					at.addLog(fmt.Sprintf("💾 Đã lưu %d tokens vào file", len(validTokens)))
					// Update token info immediately after saving
					at.updateTokenInfo()
				}
			}
		}

		// Update progress
		at.gui.updateUI <- func() {
			at.addLog(fmt.Sprintf("📊 Tiến độ: %d/%d accounts | Success: %d | Fail: %d",
				end, len(at.accounts), successCount, failCount))
		}

		// Rest between batches (except last batch)
		if end < len(at.accounts) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				// Continue to next batch
			}
		}
	}

	// Final summary
	at.gui.updateUI <- func() {
		at.addLog("🎉 HOÀN THÀNH TOKEN EXTRACTION!")
		at.addLog(fmt.Sprintf("📈 Kết quả: Success: %d | Fail: %d | Total: %d",
			successCount, failCount, len(at.accounts)))

		if successCount > 0 {
			at.addLog("✅ Có thể bắt đầu crawl emails với tokens đã có!")
		}

		// Final update of token info
		at.updateTokenInfo()
	}
}

func (at *AccountsTab) CleanAllAccounts() {
	dialog.ShowConfirm("Clean All", "Xoá hết account?", func(ok bool) {
		if ok {
			at.accounts = []models.Account{}
			at.accountData = binding.NewStringList()
			at.setupAccountsList()
			at.updateStats()
			at.addLog("🗑️ Đã xoá hết accounts.")
		}
	}, at.gui.window)
}

func (at *AccountsTab) addLog(msg string) {
	ts := time.Now().Format("15:04:05")
	logEntry := fmt.Sprintf("[%s] %s", ts, msg)
	at.logBuffer = append(at.logBuffer, logEntry)

	// Keep only last 200 entries
	if len(at.logBuffer) > 200 {
		at.logBuffer = at.logBuffer[len(at.logBuffer)-200:]
	}

	// Update display
	displayText := "```\n" + strings.Join(at.logBuffer, "\n") + "\n```"
	at.logText.ParseMarkdown(displayText)
}

func (at *AccountsTab) ImportAccounts() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()
		raw, err := io.ReadAll(reader)
		if err != nil {
			at.gui.updateUI <- func() {
				dialog.ShowError(fmt.Errorf("Failed to read file: %v", err), at.gui.window)
			}
			return
		}
		lines := strings.Split(string(raw), "\n")
		imported := 0
		skipped := 0
		emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "|")
			if len(parts) < 2 {
				continue
			}
			email := strings.TrimSpace(parts[0])
			password := strings.TrimSpace(parts[1])
			if !emailRegex.MatchString(email) || len(password) < 6 {
				continue
			}
			exists := false
			for _, account := range at.accounts {
				if account.Email == email {
					exists = true
					skipped++
					break
				}
			}
			if !exists {
				at.accounts = append(at.accounts, models.Account{Email: email, Password: password})
				at.accountData.Append(fmt.Sprintf("%s|%s", email, password))
				imported++
			}
		}
		at.gui.updateUI <- func() {
			at.accountsList.Refresh()
			at.updateStats()
			message := fmt.Sprintf("Imported: %d | Skipped: %d", imported, skipped)
			dialog.ShowInformation("Import Results", message, at.gui.window)
			at.gui.updateStatus(fmt.Sprintf("Imported %d accounts", imported))
			at.addLog(fmt.Sprintf("📥 Import: %d accounts thành công, %d bị bỏ qua", imported, skipped))
		}
	}, at.gui.window)
}

func (at *AccountsTab) LoadAccounts() {
	accountStorage := storageInternal.NewAccountStorage()
	accounts, err := accountStorage.LoadAccounts("accounts.txt")
	if err != nil {
		if _, err := os.Stat("accounts.txt"); os.IsNotExist(err) {
			sampleContent := `# Microsoft Teams Accounts
# Format: email|password
user1@company.com|password123
`
			os.WriteFile("accounts.txt", []byte(sampleContent), 0644)
		}
		at.gui.updateUI <- func() {
			at.gui.updateStatus("No accounts file found")
		}
		return
	}
	at.accounts = []models.Account{}
	at.accountData = binding.NewStringList()
	at.setupAccountsList()
	for _, account := range accounts {
		at.accounts = append(at.accounts, account)
		at.accountData.Append(fmt.Sprintf("%s|%s", account.Email, account.Password))
	}
	at.gui.updateUI <- func() {
		at.accountsList.Refresh()
		at.updateStats()
		at.gui.updateStatus(fmt.Sprintf("Loaded %d accounts", len(accounts)))
		at.addLog(fmt.Sprintf("📂 Loaded %d accounts từ file", len(accounts)))
	}
}

func (at *AccountsTab) SaveAccounts() {
	if len(at.accounts) == 0 {
		return
	}
	var lines []string
	lines = append(lines, "# Microsoft Teams Accounts")
	lines = append(lines, "# Format: email|password")
	lines = append(lines, fmt.Sprintf("# Last saved: %s", time.Now().Format("2006-01-02 15:04:05")))
	lines = append(lines, "")
	for _, account := range at.accounts {
		lines = append(lines, fmt.Sprintf("%s|%s", account.Email, account.Password))
	}
	content := strings.Join(lines, "\n")
	err := os.WriteFile("accounts.txt", []byte(content), 0644)
	if err != nil {
		at.gui.updateUI <- func() {
			at.gui.updateStatus(fmt.Sprintf("Failed to save: %v", err))
		}
		return
	}
	at.gui.updateUI <- func() {
		at.gui.updateStatus(fmt.Sprintf("Saved %d accounts", len(at.accounts)))
		at.addLog(fmt.Sprintf("💾 Saved %d accounts to file", len(at.accounts)))
	}
}

func (at *AccountsTab) RefreshAccountsList() {
	at.LoadAccounts()
	// Also refresh token info when refreshing accounts
	at.updateTokenInfo()
}

func (at *AccountsTab) isValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

func (at *AccountsTab) getAccountStatus(email string) string {
	for _, account := range at.accounts {
		if account.Email == email {
			if len(account.Password) >= 6 && at.isValidEmail(account.Email) {
				return "Ready"
			} else {
				return "Failed"
			}
		}
	}
	return "Unknown"
}

func (at *AccountsTab) updateStats() {
	total := len(at.accounts)
	used := 0
	failed := 0
	for _, account := range at.accounts {
		if len(account.Password) < 6 || !at.isValidEmail(account.Email) {
			failed++
		}
	}
	available := total - used - failed
	at.totalLabel.SetText(fmt.Sprintf("Total: %d", total))
	at.usedLabel.SetText(fmt.Sprintf("Used: %d", used))
	at.remainingLabel.SetText(fmt.Sprintf("Available: %d", available))
}

func (at *AccountsTab) GetAccounts() []models.Account {
	return at.accounts
}
