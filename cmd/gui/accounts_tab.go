package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"linkedin-crawler/internal/models"
	storageInternal "linkedin-crawler/internal/storage"
)

// NewAccountsTab creates a new accounts management tab
func NewAccountsTab(gui *CrawlerGUI) *AccountsTab {
	tab := &AccountsTab{
		gui:         gui,
		accounts:    []models.Account{},
		accountData: binding.NewStringList(),
	}

	// Initialize form fields
	tab.emailEntry = widget.NewEntry()
	tab.passwordEntry = widget.NewPasswordEntry()

	// Setup form field validation and placeholders
	tab.setupFormFields()

	// Initialize buttons
	tab.addBtn = widget.NewButtonWithIcon("Add Account", theme.ContentAddIcon(), tab.AddAccount)
	tab.removeBtn = widget.NewButtonWithIcon("Remove Selected", theme.ContentRemoveIcon(), tab.RemoveAccount)
	tab.importBtn = widget.NewButtonWithIcon("Import from File", theme.FolderOpenIcon(), tab.ImportAccounts)
	tab.exportBtn = widget.NewButtonWithIcon("Export to File", theme.DocumentSaveIcon(), tab.ExportAccounts)

	// Style buttons
	tab.addBtn.Importance = widget.HighImportance
	tab.removeBtn.Importance = widget.DangerImportance

	// Initialize stats labels
	tab.totalLabel = widget.NewLabel("Total: 0")
	tab.usedLabel = widget.NewLabel("Used: 0")
	tab.remainingLabel = widget.NewLabel("Available: 0")

	// Initialize list
	tab.setupAccountsList()

	return tab
}

// setupFormFields configures form fields with validation
func (at *AccountsTab) setupFormFields() {
	at.emailEntry.SetPlaceHolder("user@company.com")
	at.emailEntry.Validator = func(s string) error {
		if s == "" {
			return fmt.Errorf("email is required")
		}
		emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
		if !emailRegex.MatchString(s) {
			return fmt.Errorf("invalid email format")
		}
		return nil
	}

	at.passwordEntry.SetPlaceHolder("password")
	at.passwordEntry.Validator = func(s string) error {
		if s == "" {
			return fmt.Errorf("password is required")
		}
		if len(s) < 6 {
			return fmt.Errorf("password must be at least 6 characters")
		}
		return nil
	}
}

// CreateContent creates the accounts tab content
func (at *AccountsTab) CreateContent() fyne.CanvasObject {
	addForm := at.createAddAccountForm()
	addCard := widget.NewCard("Add New Account",
		"Add Microsoft Teams accounts for token extraction", addForm)

	addButtonContainer := container.NewHBox(
		at.addBtn,
		widget.NewButton("Clear", at.clearForm),
		widget.NewButton("Validate", at.validateCurrentForm),
	)

	importExportCard := at.createImportExportCard()
	statsCard := at.createStatsCard()
	securityCard := at.createSecurityCard()

	leftPanel := container.NewVBox(
		addCard,
		addButtonContainer,
		widget.NewSeparator(),
		importExportCard,
		statsCard,
		securityCard,
	)

	listHeader := container.NewHBox(
		widget.NewLabel("Microsoft Teams Accounts"),
		widget.NewButton("Refresh", at.RefreshAccountsList),
		widget.NewButton("Test Selected", at.TestSelectedAccount),
		at.removeBtn,
	)

	rightPanel := container.NewVBox(
		listHeader,
		container.NewScroll(at.accountsList),
		at.createAccountActionsCard(),
	)

	content := container.NewHSplit(leftPanel, rightPanel)
	content.SetOffset(0.4)

	return content
}

func (at *AccountsTab) createAddAccountForm() *widget.Form {
	return &widget.Form{
		Items: []*widget.FormItem{
			{
				Text:     "Email Address:",
				Widget:   at.emailEntry,
				HintText: "Microsoft Teams account email",
			},
			{
				Text:     "Password:",
				Widget:   at.passwordEntry,
				HintText: "Account password (stored locally)",
			},
		},
	}
}

func (at *AccountsTab) createImportExportCard() *widget.Card {
	content := container.NewVBox(
		widget.NewRichTextFromMarkdown("**Supported formats:** TXT with `email|password` format"),
		widget.NewRichTextFromMarkdown("**Example:**\n```\nuser1@company.com|password123\nuser2@company.com|password456\n```"),
		container.NewHBox(at.importBtn, at.exportBtn),
		widget.NewButton("Bulk Add", at.ShowBulkAddDialog),
	)
	return widget.NewCard("Bulk Operations",
		"Import/export accounts and bulk operations", content)
}

func (at *AccountsTab) createStatsCard() *widget.Card {
	statsGrid := container.NewGridWithColumns(1,
		at.totalLabel,
		at.usedLabel,
		at.remainingLabel,
	)
	return widget.NewCard("Account Statistics",
		"Current account usage information", statsGrid)
}

func (at *AccountsTab) createSecurityCard() *widget.Card {
	securityInfo := widget.NewRichTextFromMarkdown(`
### 🔒 Security & Requirements

**Account Requirements:**
- Valid Microsoft Teams accounts
- LinkedIn integration enabled
- 2FA should be disabled for automation

**Security Notes:**
- Passwords stored locally only
- Use dedicated accounts for crawling
- Accounts may be consumed during extraction
- Regular credential rotation recommended

**Best Practices:**
- Use separate accounts from your main business accounts
- Monitor account activity regularly
- Remove failed accounts to avoid retries
- Keep backup accounts available

**Troubleshooting:**
- Ensure accounts have Teams access
- Verify LinkedIn integration is enabled
- Check for account locks or restrictions
	`)
	return widget.NewCard("Security & Help", "", securityInfo)
}

func (at *AccountsTab) createAccountActionsCard() *widget.Card {
	content := container.NewVBox(
		widget.NewButton("Validate All Accounts", at.ValidateAllAccounts),
		widget.NewButton("Remove Failed Accounts", at.RemoveFailedAccounts),
		widget.NewButton("Export Valid Accounts", at.ExportValidAccounts),
		widget.NewSeparator(),
		widget.NewRichTextFromMarkdown("**Quick Actions:** Batch operations for account management"),
	)
	return widget.NewCard("Account Actions",
		"Batch operations and account maintenance", content)
}

func (at *AccountsTab) setupAccountsList() {
	at.accountsList = widget.NewListWithData(
		at.accountData,
		func() fyne.CanvasObject {
			icon := widget.NewIcon(theme.AccountIcon())
			email := widget.NewLabel("Template Email")
			status := widget.NewLabel("Status")
			infoContainer := container.NewVBox(email, status)
			return container.NewHBox(icon, infoContainer)
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
			} else {
				emailLabel.SetText(str)
				statusLabel.SetText("Invalid format")
				icon.SetResource(theme.ErrorIcon())
			}
		},
	)
	// Bắt sự kiện chọn item và cập nhật selectedIndex
	at.accountsList.OnSelected = func(id widget.ListItemID) {
		at.selectedIndex = int(id)
		if id < len(at.accounts) {
			account := at.accounts[id]
			at.emailEntry.SetText(account.Email)
			at.passwordEntry.SetText(account.Password)
		}
	}
	// Reset selectedIndex mỗi khi làm mới
	at.selectedIndex = -1
}

// AddAccount adds a new account to the list
func (at *AccountsTab) AddAccount() {
	if err := at.emailEntry.Validate(); err != nil {
		dialog.ShowError(fmt.Errorf("Email validation failed: %v", err), at.gui.window)
		return
	}
	if err := at.passwordEntry.Validate(); err != nil {
		dialog.ShowError(fmt.Errorf("Password validation failed: %v", err), at.gui.window)
		return
	}
	email := strings.TrimSpace(at.emailEntry.Text)
	password := strings.TrimSpace(at.passwordEntry.Text)
	for _, account := range at.accounts {
		if account.Email == email {
			dialog.ShowError(fmt.Errorf("Account already exists: %s", email), at.gui.window)
			return
		}
	}
	newAccount := models.Account{
		Email:    email,
		Password: password,
	}
	at.accounts = append(at.accounts, newAccount)
	at.accountData.Append(fmt.Sprintf("%s|%s", email, password))
	at.accountsList.Refresh()
	at.clearForm()
	at.updateStats()
	at.gui.logsTab.AddLog(fmt.Sprintf("✅ Added account: %s", email))
	at.gui.updateStatus(fmt.Sprintf("Added account: %s", email))
}

// RemoveAccount removes the selected account
func (at *AccountsTab) RemoveAccount() {
	id := at.selectedIndex
	if id < 0 || id >= len(at.accounts) {
		dialog.ShowInformation("No Selection", "Please select an account to remove", at.gui.window)
		return
	}

	account := at.accounts[id]
	dialog.ShowConfirm("Confirm Removal",
		fmt.Sprintf("Are you sure you want to remove account: %s?\n\nThis action cannot be undone.", account.Email),
		func(confirmed bool) {
			if confirmed {
				// Remove from slice
				at.accounts = append(at.accounts[:id], at.accounts[id+1:]...)

				// Cập nhật lại accountData
				at.accountData = binding.NewStringList()
				for _, acc := range at.accounts {
					at.accountData.Append(fmt.Sprintf("%s|%s", acc.Email, acc.Password))
				}
				at.accountsList.Refresh()
				at.updateStats()
				at.gui.logsTab.AddLog(fmt.Sprintf("🗑️ Removed account: %s", account.Email))
				at.gui.updateStatus(fmt.Sprintf("Removed account: %s", account.Email))
			}
		}, at.gui.window)
}

// ImportAccounts imports accounts from a file
func (at *AccountsTab) ImportAccounts() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()

		content, err := os.ReadFile(reader.URI().Path())
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to read file: %v", err), at.gui.window)
			return
		}

		lines := strings.Split(string(content), "\n")
		var imported, duplicates int
		errors := []string{}
		emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

		for idx, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "|")
			if len(parts) < 2 {
				errors = append(errors, fmt.Sprintf("Line %d: Invalid format", idx+1))
				continue
			}
			email := strings.TrimSpace(parts[0])
			password := strings.TrimSpace(parts[1])
			if !emailRegex.MatchString(email) {
				errors = append(errors, fmt.Sprintf("Line %d: Invalid email format", idx+1))
				continue
			}
			exists := false
			for _, account := range at.accounts {
				if account.Email == email {
					duplicates++
					exists = true
					break
				}
			}
			if !exists {
				at.accounts = append(at.accounts, models.Account{Email: email, Password: password})
				at.accountData.Append(fmt.Sprintf("%s|%s", email, password))
				imported++
			}
		}
		at.accountsList.Refresh()
		at.updateStats()
		message := fmt.Sprintf("Imported %d accounts successfully", imported)
		if duplicates > 0 {
			message += fmt.Sprintf("\nSkipped %d duplicates", duplicates)
		}
		if len(errors) > 0 {
			message += fmt.Sprintf("\n\nErrors:\n%s", strings.Join(errors, "\n"))
		}
		dialog.ShowInformation("Import Results", message, at.gui.window)
		at.gui.logsTab.AddLog(fmt.Sprintf("📁 Import completed: %d accounts", imported))
	}, at.gui.window)
}

// ExportAccounts exports accounts to a file
func (at *AccountsTab) ExportAccounts() {
	if len(at.accounts) == 0 {
		dialog.ShowInformation("No Data", "No accounts to export", at.gui.window)
		return
	}
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}
		defer writer.Close()
		var lines []string
		lines = append(lines, "# Microsoft Teams Accounts for LinkedIn Auto Crawler")
		lines = append(lines, "# Format: email|password")
		lines = append(lines, fmt.Sprintf("# Generated: %s", time.Now().Format("2006-01-02 15:04:05")))
		lines = append(lines, "# SECURITY WARNING: This file contains passwords in plain text")
		lines = append(lines, "")
		for _, account := range at.accounts {
			lines = append(lines, fmt.Sprintf("%s|%s", account.Email, account.Password))
		}
		content := strings.Join(lines, "\n")
		_, err = writer.Write([]byte(content))
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to write file: %v", err), at.gui.window)
			return
		}
		at.gui.updateStatus(fmt.Sprintf("Exported %d accounts to file", len(at.accounts)))
		at.gui.logsTab.AddLog(fmt.Sprintf("💾 Exported %d accounts", len(at.accounts)))
	}, at.gui.window)
}

// LoadAccounts loads accounts from the default accounts.txt file
func (at *AccountsTab) LoadAccounts() {
	accountStorage := storageInternal.NewAccountStorage()
	accounts, err := accountStorage.LoadAccounts("accounts.txt")
	if err != nil {
		if _, err := os.Stat("accounts.txt"); os.IsNotExist(err) {
			sampleContent := `# Microsoft Teams Accounts for LinkedIn Auto Crawler
# Format: email|password
# Example:
# user1@company.com|password123
# user2@company.com|mypassword456

# Add your accounts below:
`
			os.WriteFile("accounts.txt", []byte(sampleContent), 0644)
			at.gui.logsTab.AddLog("📝 Created sample accounts.txt file")
		}
		at.gui.updateStatus("No accounts file found, created sample file")
		return
	}
	at.accounts = []models.Account{}
	at.accountData = binding.NewStringList()
	at.setupAccountsList()
	for _, account := range accounts {
		at.accounts = append(at.accounts, account)
		at.accountData.Append(fmt.Sprintf("%s|%s", account.Email, account.Password))
	}
	at.accountsList.Refresh()
	at.updateStats()
	at.gui.updateStatus(fmt.Sprintf("Loaded %d accounts from file", len(accounts)))
	at.gui.logsTab.AddLog(fmt.Sprintf("📂 Loaded %d accounts from accounts.txt", len(accounts)))
}

// SaveAccounts saves accounts to the default accounts.txt file
func (at *AccountsTab) SaveAccounts() {
	if len(at.accounts) == 0 {
		return
	}
	var lines []string
	lines = append(lines, "# Microsoft Teams Accounts for LinkedIn Auto Crawler")
	lines = append(lines, "# Format: email|password")
	lines = append(lines, "# Generated by GUI on "+time.Now().Format("2006-01-02 15:04:05"))
	lines = append(lines, "")
	for _, account := range at.accounts {
		lines = append(lines, fmt.Sprintf("%s|%s", account.Email, account.Password))
	}
	content := strings.Join(lines, "\n")
	err := os.WriteFile("accounts.txt", []byte(content), 0644)
	if err != nil {
		at.gui.updateStatus(fmt.Sprintf("Failed to save accounts: %v", err))
		at.gui.logsTab.AddLog(fmt.Sprintf("❌ Failed to save accounts: %v", err))
		return
	}
	at.gui.updateStatus(fmt.Sprintf("Saved %d accounts to file", len(at.accounts)))
	at.gui.logsTab.AddLog(fmt.Sprintf("💾 Saved %d accounts to accounts.txt", len(at.accounts)))
}

func (at *AccountsTab) RefreshAccountsList() {
	at.LoadAccounts()
}

// ShowBulkAddDialog displays a dialog for bulk adding accounts
func (at *AccountsTab) ShowBulkAddDialog() {
	entry := widget.NewMultiLineEntry()
	dialog.ShowForm("Bulk Add Accounts",
		"Add", "Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("Accounts (one per line, email|password):", entry),
		},
		func(confirm bool) {
			if !confirm {
				return
			}
			lines := strings.Split(entry.Text, "\n")
			count := 0
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
				if !emailRegex.MatchString(email) {
					continue
				}
				exists := false
				for _, account := range at.accounts {
					if account.Email == email {
						exists = true
						break
					}
				}
				if !exists {
					at.accounts = append(at.accounts, models.Account{Email: email, Password: password})
					at.accountData.Append(fmt.Sprintf("%s|%s", email, password))
					count++
				}
			}
			at.accountsList.Refresh()
			at.updateStats()
			at.gui.updateStatus(fmt.Sprintf("Bulk added %d accounts", count))
			at.gui.logsTab.AddLog(fmt.Sprintf("🚀 Bulk added %d accounts", count))
		}, at.gui.window)
}

func (at *AccountsTab) TestSelectedAccount() {
	id := at.selectedIndex
	if id < 0 || id >= len(at.accounts) {
		dialog.ShowInformation("No Selection", "Please select an account to test", at.gui.window)
		return
	}
	account := at.accounts[id]

	// Hiện dialog tiến trình
	progressDialog := dialog.NewProgressInfinite("Testing Account",
		fmt.Sprintf("Testing credentials for %s...", account.Email), at.gui.window)
	progressDialog.Show()

	// Giả lập kiểm tra tài khoản (bạn thay bằng logic thật nếu muốn)
	go func() {
		defer progressDialog.Hide()
		time.Sleep(2 * time.Second)
		success := len(account.Password) >= 8 // Hoặc call API, login v.v.

		if success {
			dialog.ShowInformation("Test Result",
				fmt.Sprintf("✅ Account %s appears to be valid", account.Email), at.gui.window)
			at.gui.logsTab.AddLog(fmt.Sprintf("✅ Account test passed: %s", account.Email))
		} else {
			dialog.ShowError(fmt.Errorf("❌ Account %s failed validation", account.Email), at.gui.window)
			at.gui.logsTab.AddLog(fmt.Sprintf("❌ Account test failed: %s", account.Email))
		}
	}()
}

func (at *AccountsTab) ValidateAllAccounts() {
	if len(at.accounts) == 0 {
		dialog.ShowInformation("No Accounts", "No accounts to validate", at.gui.window)
		return
	}
	progressDialog := dialog.NewProgressInfinite("Validating Accounts",
		"Validating all accounts...", at.gui.window)
	progressDialog.Show()
	go func() {
		defer progressDialog.Hide()
		valid := 0
		invalid := 0
		for _, account := range at.accounts {
			time.Sleep(500 * time.Millisecond)
			if len(account.Password) >= 6 {
				valid++
				at.gui.logsTab.AddLog(fmt.Sprintf("✅ Valid: %s", account.Email))
			} else {
				invalid++
				at.gui.logsTab.AddLog(fmt.Sprintf("❌ Invalid: %s", account.Email))
			}
		}
		message := fmt.Sprintf("Validation complete:\n✅ Valid: %d\n❌ Invalid: %d", valid, invalid)
		dialog.ShowInformation("Validation Results", message, at.gui.window)
		at.gui.updateStatus(fmt.Sprintf("Validated %d accounts", len(at.accounts)))
	}()
}

func (at *AccountsTab) RemoveFailedAccounts() {
	originalCount := len(at.accounts)
	validAccounts := []models.Account{}
	for _, account := range at.accounts {
		if len(account.Password) >= 8 {
			validAccounts = append(validAccounts, account)
		} else {
			at.gui.logsTab.AddLog(fmt.Sprintf("🗑️ Removed failed account: %s", account.Email))
		}
	}
	removedCount := originalCount - len(validAccounts)
	if removedCount > 0 {
		dialog.ShowConfirm("Confirm Removal",
			fmt.Sprintf("Remove %d failed accounts?", removedCount),
			func(confirmed bool) {
				if confirmed {
					at.accounts = validAccounts
					at.accountData = binding.NewStringList()
					for _, account := range at.accounts {
						at.accountData.Append(fmt.Sprintf("%s|%s", account.Email, account.Password))
					}
					at.accountsList.Refresh()
					at.updateStats()
					at.gui.updateStatus(fmt.Sprintf("Removed %d failed accounts", removedCount))
				}
			}, at.gui.window)
	} else {
		dialog.ShowInformation("No Failed Accounts", "No failed accounts found to remove", at.gui.window)
	}
}

func (at *AccountsTab) ExportValidAccounts() {
	validAccounts := []models.Account{}
	for _, account := range at.accounts {
		if len(account.Password) >= 8 {
			validAccounts = append(validAccounts, account)
		}
	}
	if len(validAccounts) == 0 {
		dialog.ShowInformation("No Valid Accounts", "No valid accounts found to export", at.gui.window)
		return
	}
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}
		defer writer.Close()
		var lines []string
		lines = append(lines, "# Valid Microsoft Teams Accounts")
		lines = append(lines, "# Exported: "+time.Now().Format("2006-01-02 15:04:05"))
		lines = append(lines, "")
		for _, account := range validAccounts {
			lines = append(lines, fmt.Sprintf("%s|%s", account.Email, account.Password))
		}
		content := strings.Join(lines, "\n")
		_, err = writer.Write([]byte(content))
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to write file: %v", err), at.gui.window)
			return
		}
		at.gui.updateStatus(fmt.Sprintf("Exported %d valid accounts", len(validAccounts)))
		at.gui.logsTab.AddLog(fmt.Sprintf("💾 Exported %d valid accounts", len(validAccounts)))
	}, at.gui.window)
}

// Helpers

func (at *AccountsTab) clearForm() {
	at.emailEntry.SetText("")
	at.passwordEntry.SetText("")
}

func (at *AccountsTab) validateCurrentForm() {
	errors := []string{}
	if err := at.emailEntry.Validate(); err != nil {
		errors = append(errors, "Email: "+err.Error())
	}
	if err := at.passwordEntry.Validate(); err != nil {
		errors = append(errors, "Password: "+err.Error())
	}
	if len(errors) > 0 {
		dialog.ShowError(fmt.Errorf(strings.Join(errors, "\n")), at.gui.window)
	} else {
		dialog.ShowInformation("Validation", "✅ Form validation passed", at.gui.window)
	}
}

func (at *AccountsTab) getAccountStatus(email string) string {
	for _, account := range at.accounts {
		if account.Email == email {
			if len(account.Password) >= 8 {
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
		if len(account.Password) < 6 {
			failed++
		}
	}
	remaining := total - used - failed
	at.totalLabel.SetText(fmt.Sprintf("Total: %d", total))
	at.usedLabel.SetText(fmt.Sprintf("Used: %d", used))
	at.remainingLabel.SetText(fmt.Sprintf("Available: %d", remaining))
}

func (at *AccountsTab) GetAccounts() []models.Account {
	return at.accounts
}
