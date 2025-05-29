package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// NewResultsTab creates a new results tab with auto-refresh functionality and deduplication
func NewResultsTab(gui *CrawlerGUI) *ResultsTab {
	tab := &ResultsTab{
		gui:     gui,
		results: []CrawlerResult{},
	}

	// Initialize buttons
	tab.refreshBtn = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), tab.RefreshResults)
	tab.exportBtn = widget.NewButtonWithIcon("Export", theme.DocumentSaveIcon(), tab.ExportResults)
	tab.clearBtn = widget.NewButtonWithIcon("Clear", theme.DeleteIcon(), tab.ClearResults)

	// Initialize filter
	tab.filterEntry = widget.NewEntry()
	tab.filterEntry.SetPlaceHolder("Filter by email, name...")
	tab.filterEntry.OnChanged = tab.applyFilter

	// Auto-refresh toggle
	tab.autoRefreshCheck = widget.NewCheck("Auto-refresh (5s)", func(checked bool) {
		tab.autoRefresh = checked
		if checked {
			tab.startAutoRefresh()
			tab.gui.updateStatus("Auto-refresh enabled (5s)")
		} else {
			tab.stopAutoRefresh()
			tab.gui.updateStatus("Auto-refresh disabled")
		}
	})
	tab.autoRefreshCheck.SetChecked(true) // Default enabled
	tab.autoRefresh = true

	// Initialize table
	tab.setupResultsTable()

	// Initialize summary
	tab.summaryCard = widget.NewCard("Summary", "", widget.NewLabel("No results yet"))

	// Start auto-refresh by default
	tab.startAutoRefresh()

	return tab
}

// CreateContent creates the results tab content
func (rt *ResultsTab) CreateContent() fyne.CanvasObject {
	// Controls section
	sortSelect := widget.NewSelect([]string{"Timestamp", "Email", "Name"}, func(value string) {
		rt.sortResults(value)
	})
	sortSelect.SetSelected("Timestamp")

	showSelect := widget.NewSelect([]string{"All", "With LinkedIn", "Without LinkedIn"}, func(value string) {
		rt.filterByStatus(value)
	})
	showSelect.SetSelected("All")

	// Control buttons row
	controlsRow1 := container.NewHBox(
		rt.refreshBtn,
		rt.exportBtn,
		rt.clearBtn,
		widget.NewSeparator(),
		rt.autoRefreshCheck,
		widget.NewSeparator(),
		widget.NewButton("Remove Duplicates", rt.RemoveDuplicates), // NEW: Remove duplicates button
	)

	// Filter and sort row
	controlsRow2 := container.NewHBox(
		widget.NewLabel("Filter:"),
		rt.filterEntry,
		widget.NewSeparator(),
		widget.NewLabel("Sort:"),
		sortSelect,
		widget.NewSeparator(),
		widget.NewLabel("Show:"),
		showSelect,
	)

	// Combined controls
	controls := container.NewVBox(
		controlsRow1,
		controlsRow2,
	)

	// Table section with scroll
	tableContainer := container.NewBorder(
		controls, nil, nil, nil,
		container.NewScroll(rt.resultsTable),
	)

	// Summary section
	rt.updateSummary()

	// Main layout
	content := container.NewBorder(
		nil, rt.summaryCard, nil, nil,
		tableContainer,
	)

	return content
}

// startAutoRefresh starts the auto-refresh timer
func (rt *ResultsTab) startAutoRefresh() {
	if rt.refreshTicker != nil {
		rt.refreshTicker.Stop()
	}

	rt.refreshTicker = time.NewTicker(5 * time.Second)
	go func() {
		defer func() {
			if rt.refreshTicker != nil {
				rt.refreshTicker.Stop()
			}
		}()

		for {
			select {
			case <-rt.refreshTicker.C:
				if rt.autoRefresh {
					rt.gui.updateUI <- func() {
						rt.RefreshResults()
					}
				}
			case <-rt.gui.ctx.Done():
				return
			}
		}
	}()
}

// stopAutoRefresh stops the auto-refresh timer
func (rt *ResultsTab) stopAutoRefresh() {
	if rt.refreshTicker != nil {
		rt.refreshTicker.Stop()
		rt.refreshTicker = nil
	}
}

// setupResultsTable initializes the results table
func (rt *ResultsTab) setupResultsTable() {
	rt.resultsTable = widget.NewTable(
		func() (int, int) {
			return len(rt.results) + 1, 6 // +1 for header, 6 columns
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("Template")
			label.Truncation = fyne.TextTruncateEllipsis
			return label
		},
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)

			if id.Row == 0 {
				headers := []string{"Email", "Name", "LinkedIn URL", "Location", "Connections", "Status"}
				if id.Col < len(headers) {
					label.SetText(headers[id.Col])
					label.TextStyle.Bold = true
					label.Importance = widget.MediumImportance
				}
			} else if id.Row-1 < len(rt.results) {
				result := rt.results[id.Row-1]
				label.TextStyle.Bold = false

				switch id.Col {
				case 0: // Email
					label.SetText(result.Email)
					label.Importance = widget.MediumImportance
				case 1: // Name
					label.SetText(result.Name)
					label.Importance = widget.MediumImportance
				case 2: // LinkedIn URL
					if len(result.LinkedInURL) > 40 {
						label.SetText(result.LinkedInURL[:37] + "...")
					} else {
						label.SetText(result.LinkedInURL)
					}
					if result.LinkedInURL != "" && result.LinkedInURL != "N/A" {
						label.Importance = widget.SuccessImportance
					} else {
						label.Importance = widget.LowImportance
					}
				case 3: // Location
					label.SetText(result.Location)
					label.Importance = widget.MediumImportance
				case 4: // Connections
					label.SetText(result.Connections)
					label.Importance = widget.MediumImportance
				case 5: // Status
					label.SetText(result.Status)
					switch result.Status {
					case "Found":
						label.Importance = widget.SuccessImportance
					case "Not Found":
						label.Importance = widget.LowImportance
					case "Failed":
						label.Importance = widget.DangerImportance
					default:
						label.Importance = widget.MediumImportance
					}
				}
			}
		},
	)

	// Set column widths
	rt.resultsTable.SetColumnWidth(0, 200) // Email
	rt.resultsTable.SetColumnWidth(1, 150) // Name
	rt.resultsTable.SetColumnWidth(2, 250) // LinkedIn URL
	rt.resultsTable.SetColumnWidth(3, 150) // Location
	rt.resultsTable.SetColumnWidth(4, 100) // Connections
	rt.resultsTable.SetColumnWidth(5, 100) // Status
}

// RefreshResults refreshes the results from hit.txt file with DEDUPLICATION
func (rt *ResultsTab) RefreshResults() {
	oldCount := len(rt.results)

	// Use map để tránh trùng lặp
	resultsMap := make(map[string]CrawlerResult) // key = email (lowercase)
	duplicatesCount := 0

	file, err := os.Open("hit.txt")
	if err != nil {
		if !rt.autoRefresh {
			rt.gui.updateStatus("No results file found")
		}
		rt.updateSummary()
		rt.resultsTable.Refresh()
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	totalLines := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		totalLines++
		parts := strings.Split(line, "|")
		if len(parts) >= 5 {
			email := strings.TrimSpace(parts[0])
			emailKey := strings.ToLower(email) // Normalize email for deduplication

			result := CrawlerResult{
				Email:       email,
				Name:        strings.TrimSpace(parts[1]),
				LinkedInURL: strings.TrimSpace(parts[2]),
				Location:    strings.TrimSpace(parts[3]),
				Connections: strings.TrimSpace(parts[4]),
				Status:      "Found",
				Timestamp:   time.Now(),
			}

			// Check for duplicates
			if _, exists := resultsMap[emailKey]; exists {
				duplicatesCount++
				// Keep the newer/better result (can add logic here)
				continue
			}

			resultsMap[emailKey] = result
		}
	}

	// Convert map to slice
	rt.results = make([]CrawlerResult, 0, len(resultsMap))
	for _, result := range resultsMap {
		rt.results = append(rt.results, result)
	}

	// Sort by timestamp (newest first)
	sort.Slice(rt.results, func(i, j int) bool {
		return rt.results[i].Timestamp.After(rt.results[j].Timestamp)
	})

	rt.updateSummary()
	rt.resultsTable.Refresh()

	newResults := len(rt.results)

	// Update status with deduplication info
	if newResults > oldCount {
		newCount := newResults - oldCount
		statusMsg := fmt.Sprintf("Found %d new results (Total: %d)", newCount, newResults)
		if duplicatesCount > 0 {
			statusMsg += fmt.Sprintf(" | Removed %d duplicates", duplicatesCount)
		}
		rt.gui.updateStatus(statusMsg)

		// Log to emails tab if available (removed controlTab reference)
		if rt.gui.emailsTab != nil {
			rt.gui.emailsTab.LogSuccess(fmt.Sprintf("Found %d new LinkedIn profiles! Check Results tab", newCount))
		}
	} else if !rt.autoRefresh {
		statusMsg := fmt.Sprintf("Results refreshed: %d total", newResults)
		if duplicatesCount > 0 {
			statusMsg += fmt.Sprintf(" | Removed %d duplicates", duplicatesCount)
		}
		rt.gui.updateStatus(statusMsg)
	}

	// Log duplicates info if found
	if duplicatesCount > 0 && !rt.autoRefresh {
		if rt.gui.emailsTab != nil {
			rt.gui.emailsTab.LogInfo(fmt.Sprintf("Removed %d duplicate entries from results", duplicatesCount))
		}
	}
}

// RemoveDuplicates manually removes duplicates from current results
func (rt *ResultsTab) RemoveDuplicates() {
	if len(rt.results) == 0 {
		dialog.ShowInformation("No Data", "No results to process", rt.gui.window)
		return
	}

	originalCount := len(rt.results)

	// Use map để tránh trùng lặp
	resultsMap := make(map[string]CrawlerResult) // key = email (lowercase)

	for _, result := range rt.results {
		emailKey := strings.ToLower(strings.TrimSpace(result.Email))

		// Keep the first occurrence or the one with more data
		if existing, exists := resultsMap[emailKey]; exists {
			// Keep the result with more LinkedIn info or newer timestamp
			if (result.LinkedInURL != "" && result.LinkedInURL != "N/A") &&
				(existing.LinkedInURL == "" || existing.LinkedInURL == "N/A") {
				resultsMap[emailKey] = result
			} else if result.Timestamp.After(existing.Timestamp) {
				resultsMap[emailKey] = result
			}
		} else {
			resultsMap[emailKey] = result
		}
	}

	// Convert map back to slice
	rt.results = make([]CrawlerResult, 0, len(resultsMap))
	for _, result := range resultsMap {
		rt.results = append(rt.results, result)
	}

	// Sort by timestamp (newest first)
	sort.Slice(rt.results, func(i, j int) bool {
		return rt.results[i].Timestamp.After(rt.results[j].Timestamp)
	})

	duplicatesRemoved := originalCount - len(rt.results)

	rt.updateSummary()
	rt.resultsTable.Refresh()

	message := fmt.Sprintf("Removed %d duplicates\nResults: %d → %d",
		duplicatesRemoved, originalCount, len(rt.results))

	dialog.ShowInformation("Remove Duplicates", message, rt.gui.window)
	rt.gui.updateStatus(fmt.Sprintf("Removed %d duplicates from results", duplicatesRemoved))
}

// ExportResults exports results to a file with deduplication
func (rt *ResultsTab) ExportResults() {
	if len(rt.results) == 0 {
		dialog.ShowInformation("No Data", "No results to export", rt.gui.window)
		return
	}

	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}
		defer writer.Close()

		var lines []string
		lines = append(lines, "Email,Name,LinkedIn URL,Location,Connections,Status,Timestamp")

		// Use map để ensure no duplicates in export
		exportMap := make(map[string]CrawlerResult)
		for _, result := range rt.results {
			emailKey := strings.ToLower(strings.TrimSpace(result.Email))
			exportMap[emailKey] = result
		}

		for _, result := range exportMap {
			line := fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s",
				result.Email, result.Name, result.LinkedInURL,
				result.Location, result.Connections, result.Status,
				result.Timestamp.Format("2006-01-02 15:04:05"))
			lines = append(lines, line)
		}

		content := strings.Join(lines, "\n")
		_, err = writer.Write([]byte(content))
		if err != nil {
			dialog.ShowError(err, rt.gui.window)
			return
		}

		duplicatesSkipped := len(rt.results) - len(exportMap)
		statusMsg := fmt.Sprintf("Exported %d unique results to CSV", len(exportMap))
		if duplicatesSkipped > 0 {
			statusMsg += fmt.Sprintf(" (skipped %d duplicates)", duplicatesSkipped)
		}
		rt.gui.updateStatus(statusMsg)
	}, rt.gui.window)
}

// ClearResults clears all results
func (rt *ResultsTab) ClearResults() {
	if len(rt.results) == 0 {
		dialog.ShowInformation("No Data", "No results to clear", rt.gui.window)
		return
	}

	dialog.ShowConfirm("Clear Results",
		fmt.Sprintf("Clear all %d results?", len(rt.results)),
		func(confirmed bool) {
			if confirmed {
				rt.results = []CrawlerResult{}
				rt.originalResults = nil // Clear backup as well
				rt.updateSummary()
				rt.resultsTable.Refresh()
				rt.gui.updateStatus("Cleared all results")
			}
		}, rt.gui.window)
}

// updateSummary updates the summary card with real-time info and duplicate detection
func (rt *ResultsTab) updateSummary() {
	total := len(rt.results)
	withLinkedIn := 0

	// Count unique emails and detect potential duplicates
	emailMap := make(map[string]int)
	for _, result := range rt.results {
		emailKey := strings.ToLower(strings.TrimSpace(result.Email))
		emailMap[emailKey]++

		if result.LinkedInURL != "" && result.LinkedInURL != "N/A" {
			withLinkedIn++
		}
	}

	// Count duplicates (emails that appear more than once)
	duplicateEmails := 0
	for _, count := range emailMap {
		if count > 1 {
			duplicateEmails += count - 1 // Count extra occurrences as duplicates
		}
	}

	percentage := 0.0
	if total > 0 {
		percentage = float64(withLinkedIn) * 100 / float64(total)
	}

	// Get additional stats from crawler if running
	additionalStats := ""
	if rt.gui.emailsTab != nil && rt.gui.emailsTab.autoCrawler != nil {
		emailStorage, _, _ := rt.gui.emailsTab.autoCrawler.GetStorageServices()
		if emailStorage != nil {
			if stats, err := emailStorage.GetEmailStats(); err == nil {
				additionalStats = fmt.Sprintf(`
**Current Processing:**
⏳ **Pending:** %d emails
✅ **Success:** %d emails  
❌ **Failed:** %d emails
🎯 **Has LinkedIn:** %d emails
📭 **No LinkedIn:** %d emails

**Processing Rate:**
📈 **Success Rate:** %.1f%%
`, stats["pending"], stats["success"], stats["failed"], stats["has_info"], stats["no_info"],
					func() float64 {
						if stats["success"]+stats["failed"] > 0 {
							return float64(stats["success"]) * 100 / float64(stats["success"]+stats["failed"])
						}
						return 0.0
					}())
			}
		}
	}

	refreshStatus := ""
	if rt.autoRefresh {
		refreshStatus = "🔄 **Auto-refresh:** ON (every 5s)"
	} else {
		refreshStatus = "⏸️ **Auto-refresh:** OFF"
	}

	// Include duplicate detection info
	duplicateInfo := ""
	if duplicateEmails > 0 {
		duplicateInfo = fmt.Sprintf(`
⚠️ **Duplicates Detected:** %d duplicate entries found
💡 **Tip:** Click "Remove Duplicates" button to clean up
`, duplicateEmails)
	} else {
		duplicateInfo = "✅ **No Duplicates:** All entries are unique"
	}

	summaryText := fmt.Sprintf(`**LinkedIn Results Summary:**

📊 **Total Found:** %d profiles
🎯 **With LinkedIn:** %d profiles (%.1f%%)
📭 **Without LinkedIn:** %d profiles (%.1f%%)
🔍 **Unique Emails:** %d

📈 **Find Rate:** %.1f%%
📅 **Last Updated:** %s
%s
%s
%s
`, total, withLinkedIn, percentage, total-withLinkedIn, 100-percentage, len(emailMap),
		percentage, time.Now().Format("15:04:05"), refreshStatus, duplicateInfo, additionalStats)

	rt.summaryCard.SetContent(widget.NewRichTextFromMarkdown(summaryText))
}

// Filter and sort functions
func (rt *ResultsTab) applyFilter(text string) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		rt.RefreshResults()
		return
	}

	filtered := []CrawlerResult{}
	originalResults := make([]CrawlerResult, len(rt.results))
	copy(originalResults, rt.results)

	for _, r := range originalResults {
		if strings.Contains(strings.ToLower(r.Email), text) ||
			strings.Contains(strings.ToLower(r.Name), text) ||
			strings.Contains(strings.ToLower(r.Location), text) ||
			strings.Contains(strings.ToLower(r.LinkedInURL), text) {
			filtered = append(filtered, r)
		}
	}
	rt.results = filtered
	rt.updateSummary()
	rt.resultsTable.Refresh()

	rt.gui.updateStatus(fmt.Sprintf("Filtered: %d/%d results match '%s'", len(filtered), len(originalResults), text))
}

func (rt *ResultsTab) sortResults(field string) {
	switch field {
	case "Email":
		sort.Slice(rt.results, func(i, j int) bool {
			return strings.ToLower(rt.results[i].Email) < strings.ToLower(rt.results[j].Email)
		})
	case "Name":
		sort.Slice(rt.results, func(i, j int) bool {
			return strings.ToLower(rt.results[i].Name) < strings.ToLower(rt.results[j].Name)
		})
	case "Timestamp":
		sort.Slice(rt.results, func(i, j int) bool {
			return rt.results[i].Timestamp.After(rt.results[j].Timestamp) // Newest first
		})
	}
	rt.resultsTable.Refresh()
	rt.gui.updateStatus(fmt.Sprintf("Sorted by %s", field))
}

func (rt *ResultsTab) filterByStatus(status string) {
	// Save original results for restoration
	if rt.originalResults == nil {
		rt.originalResults = make([]CrawlerResult, len(rt.results))
		copy(rt.originalResults, rt.results)
	}

	filtered := []CrawlerResult{}
	sourceResults := rt.originalResults

	switch status {
	case "With LinkedIn":
		for _, r := range sourceResults {
			if r.LinkedInURL != "" && r.LinkedInURL != "N/A" {
				filtered = append(filtered, r)
			}
		}
	case "Without LinkedIn":
		for _, r := range sourceResults {
			if r.LinkedInURL == "" || r.LinkedInURL == "N/A" {
				filtered = append(filtered, r)
			}
		}
	default: // "All"
		filtered = make([]CrawlerResult, len(sourceResults))
		copy(filtered, sourceResults)
		rt.originalResults = nil // Clear saved results
	}

	rt.results = filtered
	rt.updateSummary()
	rt.resultsTable.Refresh()

	rt.gui.updateStatus(fmt.Sprintf("Showing: %s (%d results)", status, len(filtered)))
}

// GetResults returns current results
func (rt *ResultsTab) GetResults() []CrawlerResult {
	return rt.results
}

// GetResultsCount returns the number of results
func (rt *ResultsTab) GetResultsCount() int {
	return len(rt.results)
}

// GetLinkedInCount returns the number of results with LinkedIn profiles
func (rt *ResultsTab) GetLinkedInCount() int {
	count := 0
	for _, result := range rt.results {
		if result.LinkedInURL != "" && result.LinkedInURL != "N/A" {
			count++
		}
	}
	return count
}

// GetUniqueEmailsCount returns the number of unique emails
func (rt *ResultsTab) GetUniqueEmailsCount() int {
	emailMap := make(map[string]bool)
	for _, result := range rt.results {
		emailKey := strings.ToLower(strings.TrimSpace(result.Email))
		emailMap[emailKey] = true
	}
	return len(emailMap)
}

// GetDuplicatesCount returns the number of duplicate entries
func (rt *ResultsTab) GetDuplicatesCount() int {
	emailMap := make(map[string]int)
	for _, result := range rt.results {
		emailKey := strings.ToLower(strings.TrimSpace(result.Email))
		emailMap[emailKey]++
	}

	duplicates := 0
	for _, count := range emailMap {
		if count > 1 {
			duplicates += count - 1
		}
	}
	return duplicates
}

// IsAutoRefreshEnabled returns whether auto-refresh is enabled
func (rt *ResultsTab) IsAutoRefreshEnabled() bool {
	return rt.autoRefresh
}

// SetAutoRefresh enables or disables auto-refresh
func (rt *ResultsTab) SetAutoRefresh(enabled bool) {
	rt.autoRefresh = enabled
	rt.autoRefreshCheck.SetChecked(enabled)

	if enabled {
		rt.startAutoRefresh()
	} else {
		rt.stopAutoRefresh()
	}
}

// ForceRefresh forces an immediate refresh regardless of auto-refresh setting
func (rt *ResultsTab) ForceRefresh() {
	rt.RefreshResults()
}

// Cleanup method to stop auto-refresh when tab is closed
func (rt *ResultsTab) Cleanup() {
	rt.stopAutoRefresh()
}
