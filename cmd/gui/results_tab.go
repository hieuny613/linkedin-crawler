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

// ===== RESULTS TAB =====

// NewResultsTab creates a new results tab
func NewResultsTab(gui *CrawlerGUI) *ResultsTab {
	tab := &ResultsTab{
		gui:     gui,
		results: []CrawlerResult{},
	}

	// Initialize buttons
	tab.refreshBtn = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), tab.RefreshResults)
	tab.exportBtn = widget.NewButtonWithIcon("Export Results", theme.DocumentSaveIcon(), tab.ExportResults)
	tab.clearBtn = widget.NewButtonWithIcon("Clear", theme.DeleteIcon(), tab.ClearResults)

	// Initialize filter
	tab.filterEntry = widget.NewEntry()
	tab.filterEntry.SetPlaceHolder("Filter by email, name, or company...")
	tab.filterEntry.OnChanged = tab.applyFilter

	// Initialize table
	tab.setupResultsTable()

	// Initialize summary
	tab.summaryCard = widget.NewCard("Summary", "", widget.NewLabel("No results yet"))

	return tab
}

// RefreshResults refreshes the results from hit.txt file
func (rt *ResultsTab) RefreshResults() {
	rt.results = []CrawlerResult{}

	file, err := os.Open("hit.txt")
	if err != nil {
		rt.gui.updateStatus("No results file found")
		rt.updateSummary()
		rt.resultsTable.Refresh()
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) >= 5 {
			result := CrawlerResult{
				Email:       parts[0],
				Name:        parts[1],
				LinkedInURL: parts[2],
				Location:    parts[3],
				Connections: parts[4],
				Status:      "Found",
				Timestamp:   time.Now(), // Nếu có timestamp trong file thì thay ở đây
			}
			rt.results = append(rt.results, result)
		}
	}
	rt.updateSummary()
	rt.resultsTable.Refresh()
	rt.gui.updateStatus(fmt.Sprintf("Loaded %d results", len(rt.results)))
}

// ExportResults exports results to a file
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
		for _, result := range rt.results {
			line := fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s",
				result.Email, result.Name, result.LinkedInURL,
				result.Location, result.Connections, result.Status,
				result.Timestamp.Format("2006-01-02 15:04:05"))
			lines = append(lines, line)
		}
		content := strings.Join(lines, "\n")
		_, err = writer.Write([]byte(content))
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to write file: %v", err), rt.gui.window)
			return
		}
		rt.gui.updateStatus(fmt.Sprintf("Exported %d results to CSV", len(rt.results)))
	}, rt.gui.window)
}

// ClearResults clears all results
func (rt *ResultsTab) ClearResults() {
	if len(rt.results) == 0 {
		return
	}

	dialog.ShowConfirm("Confirm Clear",
		"Are you sure you want to clear all results?",
		func(confirmed bool) {
			if confirmed {
				rt.results = []CrawlerResult{}
				rt.updateSummary()
				rt.resultsTable.Refresh()
				rt.gui.updateStatus("Cleared all results")
			}
		}, rt.gui.window)
}

// updateSummary updates the summary card
func (rt *ResultsTab) updateSummary() {
	total := len(rt.results)
	withLinkedIn := 0

	for _, result := range rt.results {
		if result.LinkedInURL != "" && result.LinkedInURL != "N/A" {
			withLinkedIn++
		}
	}

	percentage := 0.0
	if total > 0 {
		percentage = float64(withLinkedIn) * 100 / float64(total)
	}

	summaryText := fmt.Sprintf(`**Results Summary:**

📊 **Total Results:** %d
🎯 **With LinkedIn:** %d (%.1f%%)
📭 **Without LinkedIn:** %d

📈 **Success Rate:** %.1f%%
📅 **Last Updated:** %s
`, total, withLinkedIn, percentage, total-withLinkedIn, percentage, time.Now().Format("15:04:05"))

	rt.summaryCard.SetContent(widget.NewRichTextFromMarkdown(summaryText))
}

// CreateContent creates the results tab content
func (rt *ResultsTab) CreateContent() fyne.CanvasObject {
	// Controls section
	sortSelect := widget.NewSelect([]string{"Timestamp", "Email", "Name", "Status"}, func(value string) {
		rt.sortResults(value)
	})
	sortSelect.SetSelected("Timestamp")

	showSelect := widget.NewSelect([]string{"All", "With LinkedIn", "Without LinkedIn", "Failed"}, func(value string) {
		rt.filterByStatus(value)
	})
	showSelect.SetSelected("All")

	controls := container.NewHBox(
		rt.refreshBtn,
		rt.exportBtn,
		rt.clearBtn,
		widget.NewSeparator(),
		widget.NewLabel("Filter:"),
		rt.filterEntry,
		widget.NewLabel("Sort:"),
		sortSelect,
		widget.NewLabel("Show:"),
		showSelect,
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

// setupResultsTable initializes the results table
func (rt *ResultsTab) setupResultsTable() {
	rt.resultsTable = widget.NewTable(
		func() (int, int) {
			return len(rt.results) + 1, 7 // +1 for header, 7 columns
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("Template")
			label.Truncation = fyne.TextTruncateEllipsis
			return label
		},
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)

			if id.Row == 0 {
				headers := []string{"Email", "Name", "LinkedIn URL", "Location", "Connections", "Status", "Timestamp"}
				if id.Col < len(headers) {
					label.SetText(headers[id.Col])
					label.TextStyle.Bold = true
				}
			} else if id.Row-1 < len(rt.results) {
				result := rt.results[id.Row-1]
				label.TextStyle.Bold = false

				switch id.Col {
				case 0:
					label.SetText(result.Email)
				case 1:
					label.SetText(result.Name)
				case 2:
					if len(result.LinkedInURL) > 50 {
						label.SetText(result.LinkedInURL[:47] + "...")
					} else {
						label.SetText(result.LinkedInURL)
					}
				case 3:
					label.SetText(result.Location)
				case 4:
					label.SetText(result.Connections)
				case 5:
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
				case 6:
					label.SetText(result.Timestamp.Format("15:04:05"))
				}
			}
		},
	)

	// Set column widths
	rt.resultsTable.SetColumnWidth(0, 200)
	rt.resultsTable.SetColumnWidth(1, 150)
	rt.resultsTable.SetColumnWidth(2, 300)
	rt.resultsTable.SetColumnWidth(3, 150)
	rt.resultsTable.SetColumnWidth(4, 100)
	rt.resultsTable.SetColumnWidth(5, 100)
	rt.resultsTable.SetColumnWidth(6, 80)

	// Optional: click LinkedIn URL to copy/log
	rt.resultsTable.OnSelected = func(id widget.TableCellID) {
		if id.Row > 0 && id.Row-1 < len(rt.results) && id.Col == 2 {
			result := rt.results[id.Row-1]
			if result.LinkedInURL != "" && result.LinkedInURL != "N/A" {
				rt.gui.logsTab.AddLog(fmt.Sprintf("🔗 LinkedIn URL: %s", result.LinkedInURL))
			}
		}
	}
}

// ----- BỔ SUNG CÁC HÀM FILTER & SORT -----

func (rt *ResultsTab) applyFilter(text string) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		rt.RefreshResults()
		return
	}

	filtered := []CrawlerResult{}
	for _, r := range rt.results {
		if strings.Contains(strings.ToLower(r.Email), text) ||
			strings.Contains(strings.ToLower(r.Name), text) ||
			strings.Contains(strings.ToLower(r.Location), text) {
			filtered = append(filtered, r)
		}
	}
	rt.results = filtered
	rt.updateSummary()
	rt.resultsTable.Refresh()
}

func (rt *ResultsTab) sortResults(field string) {
	switch field {
	case "Email":
		sort.Slice(rt.results, func(i, j int) bool { return rt.results[i].Email < rt.results[j].Email })
	case "Name":
		sort.Slice(rt.results, func(i, j int) bool { return rt.results[i].Name < rt.results[j].Name })
	case "Status":
		sort.Slice(rt.results, func(i, j int) bool { return rt.results[i].Status < rt.results[j].Status })
	case "Timestamp":
		sort.Slice(rt.results, func(i, j int) bool { return rt.results[i].Timestamp.Before(rt.results[j].Timestamp) })
	}
	rt.resultsTable.Refresh()
}

func (rt *ResultsTab) filterByStatus(status string) {
	filtered := []CrawlerResult{}
	switch status {
	case "With LinkedIn":
		for _, r := range rt.results {
			if r.LinkedInURL != "" && r.LinkedInURL != "N/A" {
				filtered = append(filtered, r)
			}
		}
	case "Without LinkedIn":
		for _, r := range rt.results {
			if r.LinkedInURL == "" || r.LinkedInURL == "N/A" {
				filtered = append(filtered, r)
			}
		}
	case "Failed":
		for _, r := range rt.results {
			if r.Status == "Failed" {
				filtered = append(filtered, r)
			}
		}
	default: // "All"
		rt.RefreshResults()
		return
	}
	rt.results = filtered
	rt.updateSummary()
	rt.resultsTable.Refresh()
}

// Optional: GetResults
func (rt *ResultsTab) GetResults() []CrawlerResult {
	return rt.results
}
