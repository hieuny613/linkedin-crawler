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

	logText   *widget.RichText
	logBuffer []string

	selectedIndex int
}

func NewAccountsTab(gui *CrawlerGUI) *AccountsTab {
	tab := &AccountsTab{
		gui:         gui,
		accounts:    []models.Account{},
		accountData: binding.NewStringList(),
	}

	tab.importBtn = widget.NewButtonWithIcon("Import", theme.FolderOpenIcon(), tab.ImportAccounts)
	tab.cleanBtn = widget.NewButtonWithIcon("Clean All", theme.DeleteIcon(), tab.CleanAllAccounts)
	tab.cleanBtn.Importance = widget.DangerImportance

	tab.startTokenBtn = widget.NewButtonWithIcon("Start Token Extract", theme.MediaPlayIcon(), tab.StartTokenExtract)
	tab.stopTokenBtn = widget.NewButtonWithIcon("Stop Token Extract", theme.MediaStopIcon(), tab.StopTokenExtract)
	tab.stopTokenBtn.Importance = widget.DangerImportance

	tab.logText = widget.NewRichText()
	tab.logText.Wrapping = fyne.TextWrapWord
	tab.logBuffer = []string{}

	tab.totalLabel = widget.NewLabel("Total: 0")
	tab.usedLabel = widget.NewLabel("Used: 0")
	tab.remainingLabel = widget.NewLabel("Available: 0")

	tab.setupAccountsList()
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
	actionsButtons := container.NewHBox(
		widget.NewButton("Validate All", at.ValidateAllAccounts),
		widget.NewButton("Remove Failed", at.RemoveFailedAccounts),
		widget.NewButton("Export Valid", at.ExportValidAccounts),
	)
	leftPanel := container.NewVBox(
		widget.NewCard("File Operations", "", fileButtons),
		widget.NewCard("Statistics", "", statsGrid),
		widget.NewCard("Quick Actions", "", actionsButtons),
		container.NewScroll(at.accountsList),
	)
	controlCard := widget.NewCard("Token Control", "", container.NewVBox(
		at.startTokenBtn,
		at.stopTokenBtn,
		widget.NewSeparator(),
		widget.NewLabel("Log:"),
		container.NewVScroll(at.logText),
	))
	controlCard.Resize(fyne.NewSize(350, 420))
	content := container.NewHSplit(leftPanel, controlCard)
	content.SetOffset(0.6)
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

func (at *AccountsTab) StartTokenExtract() {
	at.addLog("Bắt đầu lấy token...")
	// TODO: Viết code thực hiện lấy token ở đây.
}
func (at *AccountsTab) StopTokenExtract() {
	at.addLog("Dừng lấy token!")
	// TODO: Viết code dừng lấy token ở đây.
}
func (at *AccountsTab) CleanAllAccounts() {
	dialog.ShowConfirm("Clean All", "Xoá hết account?", func(ok bool) {
		if ok {
			at.accounts = []models.Account{}
			at.accountData = binding.NewStringList()
			at.setupAccountsList()
			at.updateStats()
			at.addLog("Đã xoá hết account.")
		}
	}, at.gui.window)
}
func (at *AccountsTab) addLog(msg string) {
	ts := time.Now().Format("15:04:05")
	at.logBuffer = append(at.logBuffer, fmt.Sprintf("[%s] %s", ts, msg))
	if len(at.logBuffer) > 100 {
		at.logBuffer = at.logBuffer[len(at.logBuffer)-100:]
	}
	at.logText.ParseMarkdown("```\n" + strings.Join(at.logBuffer, "\n") + "\n```")
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
	}
}

func (at *AccountsTab) RefreshAccountsList() {
	at.LoadAccounts()
}

func (at *AccountsTab) ValidateAllAccounts() {
	if len(at.accounts) == 0 {
		at.gui.updateUI <- func() {
			dialog.ShowInformation("No Accounts", "No accounts to validate", at.gui.window)
		}
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
	at.gui.updateUI <- func() {
		dialog.ShowInformation("Validation Results", message, at.gui.window)
		at.gui.updateStatus(fmt.Sprintf("Validated %d accounts", len(at.accounts)))
	}
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
					at.gui.updateUI <- func() {
						at.accountsList.Refresh()
						at.updateStats()
						at.gui.updateStatus(fmt.Sprintf("Removed %d failed accounts", removedCount))
					}
				}
			}, at.gui.window)
	} else {
		at.gui.updateUI <- func() {
			dialog.ShowInformation("No Failed Accounts", "All accounts are valid", at.gui.window)
		}
	}
}

func (at *AccountsTab) ExportValidAccounts() {
	validAccounts := []models.Account{}
	for _, account := range at.accounts {
		if len(account.Password) >= 6 && at.isValidEmail(account.Email) {
			validAccounts = append(validAccounts, account)
		}
	}
	if len(validAccounts) == 0 {
		at.gui.updateUI <- func() {
			dialog.ShowInformation("No Valid Accounts", "No valid accounts found", at.gui.window)
		}
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
			at.gui.updateUI <- func() {
				dialog.ShowError(fmt.Errorf("Failed to write file: %v", err), at.gui.window)
			}
			return
		}
		at.gui.updateUI <- func() {
			at.gui.updateStatus(fmt.Sprintf("Exported %d valid accounts", len(validAccounts)))
		}
	}, at.gui.window)
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
