package main

import (
	"fmt"
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

	// Initialize form fields
	tab.emailEntry = widget.NewEntry()
	tab.emailEntry.SetPlaceHolder("user@example.com")

	// Initialize buttons
	tab.addBtn = widget.NewButtonWithIcon("Add Email", theme.ContentAddIcon(), tab.AddEmail)
	tab.removeBtn = widget.NewButtonWithIcon("Remove Selected", theme.ContentRemoveIcon(), tab.RemoveEmail)
	tab.importBtn = widget.NewButtonWithIcon("Import from File", theme.FolderOpenIcon(), tab.ImportEmails)
	tab.exportBtn = widget.NewButtonWithIcon("Export to File", theme.DocumentSaveIcon(), tab.ExportEmails)
	tab.clearBtn = widget.NewButtonWithIcon("Clear All", theme.DeleteIcon(), tab.ClearAllEmails)

	// Style buttons
	tab.addBtn.Importance = widget.HighImportance
	tab.removeBtn.Importance = widget.DangerImportance
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
	// Add email form
	addForm := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Email Address:", Widget: et.emailEntry, HintText: "Enter target email address"},
		},
	}

	addCard := widget.NewCard("Add Target Email",
		"Add email addresses to search for LinkedIn profiles", addForm)

	// Add button container
	addButtonContainer := container.NewHBox(
		et.addBtn,
		widget.NewButton("Clear", func() {
			et.emailEntry.SetText("")
		}),
		widget.NewButton("Add Bulk", et.ShowBulkAddDialog),
	)

	// Bulk operations section
	bulkOpsCard := widget.NewCard("Bulk Operations", "",
		container.NewVBox(
			widget.NewRichTextFromMarkdown("**Supported formats:** Plain emails or CSV with email column"),
			container.NewHBox(et.importBtn, et.exportBtn),
			container.NewHBox(et.clearBtn),
		),
	)

	// Statistics section
	statsGrid := container.NewGridWithColumns(2,
		et.totalLabel, et.pendingLabel,
		et.successLabel, et.failedLabel,
		et.hasInfoLabel, et.noInfoLabel,
	)

	statsCard := widget.NewCard("Processing Statistics", "", statsGrid)

	// Email validation info
	validationCard := widget.NewCard("Email Validation", "",
		widget.NewRichTextFromMarkdown(`
**Valid Email Formats:**
- user@domain.com
- firstname.lastname@company.org
- user+tag@domain.co.uk

**Automatic Validation:**
- Format checking with regex
- Duplicate detection
- Domain validation

**Processing Status:**
- **Pending:** Not yet processed
- **Success:** Successfully processed (with or without LinkedIn data)
- **Failed:** Failed after retries
- **Has LinkedIn:** Found LinkedIn profile information
- **No LinkedIn:** No LinkedIn profile found
		`),
	)

	// Left panel
	leftPanel := container.NewVBox(
		addCard,
		addButtonContainer,
		widget.NewSeparator(),
		bulkOpsCard,
		statsCard,
		validationCard,
	)

	// Right panel with email list and controls
	listHeader := container.NewHBox(
		widget.NewLabel("Target Email Addresses"),
		widget.NewButton("Refresh Stats", et.RefreshStats),
		widget.NewButton("Refresh List", et.RefreshEmailsList),
		et.removeBtn,
	)

	// Email list with status indicators
	rightPanel := container.NewVBox(
		listHeader,
		container.NewScroll(et.emailsList),
	)

	// Main layout
	content := container.NewHSplit(leftPanel, rightPanel)
	content.SetOffset(0.4) // 40% for left, 60% for right

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

			email.Wrapping = fyne.TextWrapWord
			status.Wrapping = fyne.TextWrapWord

			// Color code status
			status.Importance = widget.MediumImportance

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
			email := et.emails[id]
			et.emailEntry.SetText(email)
		}
	}
}

// AddEmail adds a new email to the list
func (et *EmailsTab) AddEmail() {
	email := strings.TrimSpace(et.emailEntry.Text)

	// Validate email
	if !et.isValidEmail(email) {
		dialog.ShowError(fmt.Errorf("Invalid email format: %s", email), et.gui.window)
		return
	}

	// Check for duplicates
	for _, existingEmail := range et.emails {
		if existingEmail == email {
			dialog.ShowError(fmt.Errorf("Email already exists: %s", email), et.gui.window)
			return
		}
	}

	// Add email
	et.emails = append(et.emails, email)
	et.emailData.Append(email)

	// Clear form
	et.emailEntry.SetText("")

	// Update stats
	et.updateStats()

	et.gui.updateStatus(fmt.Sprintf("Added email: %s", email))
}

// RemoveEmail removes the selected email
func (et *EmailsTab) RemoveEmail() {
	id := et.selectedIndex
	if id < 0 || id >= len(et.emails) {
		dialog.ShowInformation("No Selection", "Please select an email to remove", et.gui.window)
		return
	}
	email := et.emails[id]
	dialog.ShowConfirm("Confirm Removal",
		fmt.Sprintf("Are you sure you want to remove email: %s?", email),
		func(confirmed bool) {
			if confirmed {
				et.emails = append(et.emails[:id], et.emails[id+1:]...)
				et.emailData = binding.NewStringList()
				for _, e := range et.emails {
					et.emailData.Append(e)
				}
				et.emailsList.Refresh()
				et.updateStats()
				et.gui.updateStatus(fmt.Sprintf("Removed email: %s", email))
			}
		}, et.gui.window)
}

// ShowBulkAddDialog shows dialog for adding multiple emails
func (et *EmailsTab) ShowBulkAddDialog() {
	textArea := widget.NewMultiLineEntry()
	textArea.SetPlaceHolder("Enter multiple emails, one per line:\n\nuser1@example.com\nuser2@company.org\nuser3@domain.net")
	textArea.Resize(fyne.NewSize(400, 200))

	dialog.ShowCustomConfirm("Add Multiple Emails", "Add", "Cancel",
		container.NewVBox(
			widget.NewLabel("Enter multiple email addresses (one per line):"),
			textArea,
		),
		func(confirmed bool) {
			if !confirmed {
				return
			}

			lines := strings.Split(textArea.Text, "\n")
			added := 0
			var errors []string

			for lineNum, line := range lines {
				email := strings.TrimSpace(line)
				if email == "" {
					continue
				}

				// Handle CSV format (take last column as email)
				if strings.Contains(email, ",") {
					parts := strings.Split(email, ",")
					email = strings.TrimSpace(parts[len(parts)-1])
				}

				if !et.isValidEmail(email) {
					errors = append(errors, fmt.Sprintf("Line %d: Invalid email format: %s", lineNum+1, email))
					continue
				}

				// Check for duplicates
				exists := false
				for _, existingEmail := range et.emails {
					if existingEmail == email {
						exists = true
						break
					}
				}

				if !exists {
					et.emails = append(et.emails, email)
					et.emailData.Append(email)
					added++
				}
			}

			// Update stats
			et.updateStats()

			// Show results
			message := fmt.Sprintf("Added %d emails successfully", added)
			if len(errors) > 0 {
				message += fmt.Sprintf("\n\nErrors:\n%s", strings.Join(errors, "\n"))
				dialog.ShowInformation("Import Results", message, et.gui.window)
			} else {
				et.gui.updateStatus(message)
			}
		}, et.gui.window)
}

// ImportEmails imports emails from a file
func (et *EmailsTab) ImportEmails() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()

		// Read file content
		data := make([]byte, 1024*1024) // 1MB limit
		n, err := reader.Read(data)
		if err != nil && n == 0 {
			dialog.ShowError(fmt.Errorf("Failed to read file: %v", err), et.gui.window)
			return
		}

		content := string(data[:n])
		lines := strings.Split(content, "\n")

		imported := 0
		var errors []string

		for lineNum, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			email := line

			// Handle CSV format
			if strings.Contains(line, ",") {
				parts := strings.Split(line, ",")
				// Try to find email in any column
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if et.isValidEmail(part) {
						email = part
						break
					}
				}
			}

			if !et.isValidEmail(email) {
				errors = append(errors, fmt.Sprintf("Line %d: Invalid email format: %s", lineNum+1, line))
				continue
			}

			// Check for duplicates
			exists := false
			for _, existingEmail := range et.emails {
				if existingEmail == email {
					exists = true
					break
				}
			}

			if !exists {
				et.emails = append(et.emails, email)
				et.emailData.Append(email)
				imported++
			}
		}

		// Update stats
		et.updateStats()

		// Show results
		message := fmt.Sprintf("Imported %d emails successfully", imported)
		if len(errors) > 0 {
			message += fmt.Sprintf("\n\nErrors:\n%s", strings.Join(errors, "\n"))
			dialog.ShowInformation("Import Results", message, et.gui.window)
		} else {
			et.gui.updateStatus(message)
		}

	}, et.gui.window)
}

// ExportEmails exports emails to a file
func (et *EmailsTab) ExportEmails() {
	if len(et.emails) == 0 {
		dialog.ShowInformation("No Data", "No emails to export", et.gui.window)
		return
	}

	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}
		defer writer.Close()

		// Prepare content
		var lines []string
		lines = append(lines, "# Target Email Addresses")
		lines = append(lines, "# Generated by LinkedIn Auto Crawler GUI")
		lines = append(lines, "")

		for _, email := range et.emails {
			lines = append(lines, email)
		}

		content := strings.Join(lines, "\n")

		// Write to file
		_, err = writer.Write([]byte(content))
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to write file: %v", err), et.gui.window)
			return
		}

		et.gui.updateStatus(fmt.Sprintf("Exported %d emails to file", len(et.emails)))

	}, et.gui.window)
}

// ClearAllEmails clears all emails from the list
func (et *EmailsTab) ClearAllEmails() {
	if len(et.emails) == 0 {
		return
	}

	dialog.ShowConfirm("Confirm Clear All",
		fmt.Sprintf("Are you sure you want to remove all %d emails?", len(et.emails)),
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
		// Create empty file if not exists
		if _, err := os.Stat("emails.txt"); os.IsNotExist(err) {
			sampleContent := `# Target email addresses for LinkedIn profile search
# One email per line
example@example.com
`
			os.WriteFile("emails.txt", []byte(sampleContent), 0644)
		}
		et.gui.updateStatus("No emails file found, created sample file")
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
	et.gui.updateStatus(fmt.Sprintf("Loaded %d emails from file", len(emails)))
}

// SaveEmails saves emails to the default emails.txt file
func (et *EmailsTab) SaveEmails() {
	if len(et.emails) == 0 {
		return
	}

	// Prepare content
	var lines []string
	lines = append(lines, "# Target email addresses for LinkedIn profile search")
	lines = append(lines, "# Generated by LinkedIn Auto Crawler GUI")
	lines = append(lines, "")

	for _, email := range et.emails {
		lines = append(lines, email)
	}

	content := strings.Join(lines, "\n")

	// Write to file
	err := os.WriteFile("emails.txt", []byte(content), 0644)
	if err != nil {
		et.gui.updateStatus(fmt.Sprintf("Failed to save emails: %v", err))
		return
	}

	et.gui.updateStatus(fmt.Sprintf("Saved %d emails to file", len(et.emails)))
}

// RefreshEmailsList refreshes the emails list display
func (et *EmailsTab) RefreshEmailsList() {
	et.LoadEmails()
}

// RefreshStats refreshes statistics from database
func (et *EmailsTab) RefreshStats() {
	et.updateStats()
	et.gui.updateStatus("Statistics refreshed")
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
			// This would require adding a method to get individual email status
			// For now, return pending as default
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
