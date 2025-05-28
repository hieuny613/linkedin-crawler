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

	controlCard := widget.NewCard("Email Crawl Control", "", container.NewVBox(
		et.startCrawlBtn,
		et.stopCrawlBtn,
		widget.NewSeparator(),
		widget.NewLabel("Log:"),
		container.NewVScroll(et.logText),
	))
	controlCard.Resize(fyne.NewSize(350, 420))

	content := container.NewHSplit(leftPanel, controlCard)
	content.SetOffset(0.6)
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

// ==== Control & Log ====
func (et *EmailsTab) StartCrawl() {
	et.addLog("Bắt đầu crawl email...")
	// TODO: Code thực hiện crawl email tại đây
}
func (et *EmailsTab) StopCrawl() {
	et.addLog("Dừng crawl email!")
	// TODO: Code dừng crawl email tại đây
}
func (et *EmailsTab) addLog(msg string) {
	ts := time.Now().Format("15:04:05")
	et.logBuffer = append(et.logBuffer, fmt.Sprintf("[%s] %s", ts, msg))
	if len(et.logBuffer) > 100 {
		et.logBuffer = et.logBuffer[len(et.logBuffer)-100:]
	}
	et.logText.ParseMarkdown("```\n" + strings.Join(et.logBuffer, "\n") + "\n```")
}

// ==== Các function quản lý email ====

// ImportEmails imports emails from a file - Thread-safe UI
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
		}
	}, et.gui.window)
}

// ClearAllEmails clears all emails from the list - Thread-safe UI
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
				}
			}
		}, et.gui.window)
}

// LoadEmails loads emails from the default emails.txt file - Thread-safe UI
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
	}
}

// SaveEmails saves emails to the default emails.txt file - Thread-safe UI
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
	}
}

// RefreshEmailsList refreshes the emails list display
func (et *EmailsTab) RefreshEmailsList() {
	et.LoadEmails()
}

func (et *EmailsTab) isValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

func (et *EmailsTab) getEmailStatus(email string) string {
	if et.gui.autoCrawler != nil {
		emailStorage, _, _ := et.gui.autoCrawler.GetStorageServices()
		if emailStorage != nil {
			return "Pending"
		}
	}
	return "Pending"
}

func (et *EmailsTab) updateStats() {
	total := len(et.emails)
	pending := total
	success := 0
	failed := 0
	hasInfo := 0
	noInfo := 0
	if et.gui.autoCrawler != nil {
		emailStorage, _, _ := et.gui.autoCrawler.GetStorageServices()
		if emailStorage != nil {
			if stats, err := emailStorage.GetEmailStats(); err == nil {
				pending = stats["pending"]
				success = stats["success"]
				failed = stats["failed"]
				hasInfo = stats["has_info"]
				noInfo = stats["no_info"]
			}
		}
	}
	et.totalLabel.SetText(fmt.Sprintf("Total: %d", total))
	et.pendingLabel.SetText(fmt.Sprintf("Pending: %d", pending))
	et.successLabel.SetText(fmt.Sprintf("Success: %d", success))
	et.failedLabel.SetText(fmt.Sprintf("Failed: %d", failed))
	et.hasInfoLabel.SetText(fmt.Sprintf("Has LinkedIn: %d", hasInfo))
	et.noInfoLabel.SetText(fmt.Sprintf("No LinkedIn: %d", noInfo))
}

func (et *EmailsTab) GetEmails() []string {
	return et.emails
}
