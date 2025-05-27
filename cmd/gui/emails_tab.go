package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	storageInternal "linkedin-crawler/internal/storage"
)

// NewEmailsTab creates a new emails management tab
func NewEmailsTab(gui *CrawlerGUI) *EmailsTab {
	tab := &EmailsTab{
		gui:       gui,
		emails:    []string{},
		emailData: binding.NewStringList(),
	}

	// Initialize buttons - only import and clear
	tab.importBtn = widget.NewButtonWithIcon("Import", theme.FolderOpenIcon(), tab.ImportEmails)
	tab.clearBtn = widget.NewButtonWithIcon("Clear All", theme.DeleteIcon(), tab.ClearAllEmails)

	// Style buttons
	tab.clearBtn.Importance = widget.DangerImportance

	// Initialize stats labels
	tab.totalLabel = widget.NewLabel("Total: 0")
	tab.pendingLabel = widget.NewLabel("Pending: 0")
	tab.successLabel = widget.NewLabel("Success: 0")
	tab.failedLabel = widget.NewLabel("Failed: 0")
	tab.hasInfoLabel = widget.NewLabel("Has LinkedIn: 0")
	tab.noInfoLabel = widget.NewLabel("No LinkedIn: 0")

	// Initialize list
	tab.setupEmailsList()

	return tab
}

// CreateContent creates the emails tab content
func (et *EmailsTab) CreateContent() fyne.CanvasObject {
	// File operations buttons - only import, clear, refresh
	fileButtons := container.NewHBox(
		et.importBtn,
		et.clearBtn,
		widget.NewButton("Refresh", et.RefreshEmailsList),
	)

	// Statistics section - horizontal layout for compactness
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

	// Left panel - more compact, no quick actions
	leftPanel := container.NewVBox(
		widget.NewCard("File Operations", "", fileButtons),
		widget.NewCard("Statistics", "", statsGrid),
	)

	// Right panel with email list - no Add/Remove buttons, no title
	rightPanel := container.NewVBox(
		container.NewScroll(et.emailsList),
	)

	// Main layout - adjust split for better balance
	content := container.NewHSplit(leftPanel, rightPanel)
	content.SetOffset(0.25) // 25% for left, 75% for right

	return content
}

// setupEmailsList initializes the emails list widget
func (et *EmailsTab) setupEmailsList() {
	et.emailsList = widget.NewListWithData(
		et.emailData,
		func() fyne.CanvasObject {
			icon := widget.NewIcon(theme.MailSendIcon())
			email := widget.NewLabel("Template")
			status := widget.NewLabel("Status")

			return container.NewHBox(
				icon,
				container.NewVBox(email, status),
			)
		},
		func(id binding.DataItem, obj fyne.CanvasObject) {
			str, _ := id.(binding.String).Get()

			container := obj.(*fyne.Container)
			icon := container.Objects[0].(*widget.Icon)
			infoContainer := container.Objects[1].(*fyne.Container)
			emailLabel := infoContainer.Objects[0].(*widget.Label)
			statusLabel := infoContainer.Objects[1].(*widget.Label)

			emailLabel.SetText(str)

			// Get status from database if available
			status := et.getEmailStatus(str)
			statusLabel.SetText(status)

			// Update icon based on status
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

// ImportEmails imports emails from a file - Fixed EOF error
func (et *EmailsTab) ImportEmails() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()

		// Fixed: Use io.ReadAll for proper file reading
		content, err := io.ReadAll(reader)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to read file: %v", err), et.gui.window)
			return
		}

		// Handle empty files
		if len(content) == 0 {
			dialog.ShowInformation("Empty File", "The selected file is empty", et.gui.window)
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

			// Handle CSV format
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

			// Check for duplicates
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

		et.updateStats()

		message := fmt.Sprintf("Imported: %d | Skipped: %d", imported, skipped)
		dialog.ShowInformation("Import Results", message, et.gui.window)
		et.gui.updateStatus(fmt.Sprintf("Imported %d emails", imported))
	}, et.gui.window)
}

// ClearAllEmails clears all emails from the list
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
				et.gui.updateStatus("Cleared all emails")
			}
		}, et.gui.window)
}

// LoadEmails loads emails from the default emails.txt file
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
		et.gui.updateStatus("No emails file found")
		return
	}

	// Clear existing emails
	et.emails = []string{}
	et.emailData = binding.NewStringList()
	et.setupEmailsList()

	// Load emails
	for _, email := range emails {
		et.emails = append(et.emails, email)
		et.emailData.Append(email)
	}

	et.updateStats()
	et.gui.updateStatus(fmt.Sprintf("Loaded %d emails", len(emails)))
}

// SaveEmails saves emails to the default emails.txt file
func (et *EmailsTab) SaveEmails() {
	if len(et.emails) == 0 {
		return
	}

	// Prepare content
	var lines []string
	lines = append(lines, "# Target email addresses")
	lines = append(lines, "")

	for _, email := range et.emails {
		lines = append(lines, email)
	}

	content := strings.Join(lines, "\n")

	// Write to file
	err := os.WriteFile("emails.txt", []byte(content), 0644)
	if err != nil {
		et.gui.updateStatus(fmt.Sprintf("Failed to save: %v", err))
		return
	}

	et.gui.updateStatus(fmt.Sprintf("Saved %d emails", len(et.emails)))
}

// RefreshEmailsList refreshes the emails list display
func (et *EmailsTab) RefreshEmailsList() {
	et.LoadEmails()
}

// isValidEmail validates email format
func (et *EmailsTab) isValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

// getEmailStatus gets the processing status of an email from database
func (et *EmailsTab) getEmailStatus(email string) string {
	// Try to get status from database if crawler is running
	if et.gui.autoCrawler != nil {
		emailStorage, _, _ := et.gui.autoCrawler.GetStorageServices()
		if emailStorage != nil {
			return "Pending"
		}
	}
	return "Pending"
}

// updateStats updates the statistics labels
func (et *EmailsTab) updateStats() {
	total := len(et.emails)

	// Initialize default values
	pending := total
	success := 0
	failed := 0
	hasInfo := 0
	noInfo := 0

	// Try to get real stats from database if available
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

// GetEmails returns the current list of emails
func (et *EmailsTab) GetEmails() []string {
	return et.emails
}
