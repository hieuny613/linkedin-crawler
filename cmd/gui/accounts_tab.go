package main

import (
	"fmt"
	"io"
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

	// Initialize buttons
	tab.importBtn = widget.NewButtonWithIcon("Import", theme.FolderOpenIcon(), tab.ImportAccounts)

	// Initialize stats labels
	tab.totalLabel = widget.NewLabel("Total: 0")
	tab.usedLabel = widget.NewLabel("Used: 0")
	tab.remainingLabel = widget.NewLabel("Available: 0")

	// Initialize list
	tab.setupAccountsList()

	return tab
}

// CreateContent creates the accounts tab content
func (at *AccountsTab) CreateContent() fyne.CanvasObject {
	// File operations buttons - only import and refresh
	fileButtons := container.NewHBox(
		at.importBtn,
		widget.NewButton("Refresh", at.RefreshAccountsList),
	)

	// Stats section - horizontal layout for compactness
	statsGrid := container.NewHBox(
		at.totalLabel,
		widget.NewSeparator(),
		at.usedLabel,
		widget.NewSeparator(),
		at.remainingLabel,
	)

	// Quick actions - horizontal layout
	actionsButtons := container.NewHBox(
		widget.NewButton("Validate All", at.ValidateAllAccounts),
		widget.NewButton("Remove Failed", at.RemoveFailedAccounts),
		widget.NewButton("Export Valid", at.ExportValidAccounts),
	)

	// Left panel - more compact
	leftPanel := container.NewVBox(
		widget.NewCard("File Operations", "", fileButtons),
		widget.NewCard("Statistics", "", statsGrid),
		widget.NewCard("Quick Actions", "", actionsButtons),
	)

	// Right panel with account list - no Add/Remove buttons, no title
	rightPanel := container.NewVBox(
		container.NewScroll(at.accountsList),
	)

	// Main layout - adjust split for better balance
	content := container.NewHSplit(leftPanel, rightPanel)
	content.SetOffset(0.25) // 25% for left, 75% for right

	return content
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
			}
		},
	)

	at.accountsList.OnSelected = func(id widget.ListItemID) {
		at.selectedIndex = int(id)
	}
	at.selectedIndex = -1
}

// ImportAccounts imports accounts from a file - Fixed EOF error
func (at *AccountsTab) ImportAccounts() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()

		// Đọc toàn bộ file
		raw, err := io.ReadAll(reader)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to read file: %v", err), at.gui.window)
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

			// Check duplicates
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

		at.accountsList.Refresh()
		at.updateStats()

		message := fmt.Sprintf("Imported: %d | Skipped: %d", imported, skipped)
		dialog.ShowInformation("Import Results", message, at.gui.window)
		at.gui.updateStatus(fmt.Sprintf("Imported %d accounts", imported))
	}, at.gui.window)
}

// LoadAccounts loads accounts from the default accounts.txt file
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
		at.gui.updateStatus("No accounts file found")
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
	at.gui.updateStatus(fmt.Sprintf("Loaded %d accounts", len(accounts)))
}

// SaveAccounts saves accounts to the default accounts.txt file
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
		at.gui.updateStatus(fmt.Sprintf("Failed to save: %v", err))
		return
	}

	at.gui.updateStatus(fmt.Sprintf("Saved %d accounts", len(at.accounts)))
}

func (at *AccountsTab) RefreshAccountsList() {
	at.LoadAccounts()
}

func (at *AccountsTab) ValidateAllAccounts() {
	if len(at.accounts) == 0 {
		dialog.ShowInformation("No Accounts", "No accounts to validate", at.gui.window)
		return
	}

	valid := 0
	invalid := 0

	for _, account := range at.accounts {
		if len(account.Password) >= 6 && at.isValidEmail(account.Email) {
			valid++
		} else {
			invalid++
		}
	}

	message := fmt.Sprintf("Valid: %d | Invalid: %d", valid, invalid)
	dialog.ShowInformation("Validation Results", message, at.gui.window)
	at.gui.updateStatus(fmt.Sprintf("Validated %d accounts", len(at.accounts)))
}

func (at *AccountsTab) RemoveFailedAccounts() {
	originalCount := len(at.accounts)
	validAccounts := []models.Account{}

	for _, account := range at.accounts {
		if len(account.Password) >= 6 && at.isValidEmail(account.Email) {
			validAccounts = append(validAccounts, account)
		}
	}

	removedCount := originalCount - len(validAccounts)
	if removedCount > 0 {
		dialog.ShowConfirm("Remove Failed",
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
		dialog.ShowInformation("No Failed Accounts", "All accounts are valid", at.gui.window)
	}
}

// ExportValidAccounts exports ONLY valid accounts to a file
func (at *AccountsTab) ExportValidAccounts() {
	validAccounts := []models.Account{}
	for _, account := range at.accounts {
		if len(account.Password) >= 6 && at.isValidEmail(account.Email) {
			validAccounts = append(validAccounts, account)
		}
	}

	if len(validAccounts) == 0 {
		dialog.ShowInformation("No Valid Accounts", "No valid accounts found", at.gui.window)
		return
	}

	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}
		defer writer.Close()

		var lines []string
		lines = append(lines, "# Valid Microsoft Teams Accounts")
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
	}, at.gui.window)
}

// Helper functions
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
