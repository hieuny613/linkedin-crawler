package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// NewLogsTab creates a new logs tab
func NewLogsTab(gui *CrawlerGUI) *LogsTab {
	tab := &LogsTab{
		gui:       gui,
		logBuffer: []string{},
		maxLogs:   1000,
	}

	// Initialize controls
	tab.clearBtn = widget.NewButtonWithIcon("Clear Logs", theme.DeleteIcon(), tab.ClearLogs)
	tab.saveBtn = widget.NewButtonWithIcon("Save Logs", theme.DocumentSaveIcon(), tab.SaveLogs)
	tab.autoScroll = widget.NewCheck("Auto-scroll", nil)
	tab.autoScroll.SetChecked(true)

	// Initialize log levels
	tab.levelSelect = widget.NewSelect([]string{"All", "Info", "Warning", "Error"}, nil)
	tab.levelSelect.SetSelected("All")

	// Initialize log display
	tab.logText = widget.NewRichText()
	tab.logText.Wrapping = fyne.TextWrapWord
	tab.logScroll = container.NewScroll(tab.logText)

	return tab
}

// CreateContent creates the logs tab content
func (lt *LogsTab) CreateContent() fyne.CanvasObject {
	controls := container.NewHBox(
		lt.clearBtn,
		lt.saveBtn,
		widget.NewSeparator(),
		widget.NewLabel("Level:"),
		lt.levelSelect,
		widget.NewSeparator(),
		lt.autoScroll,
	)

	logContainer := container.NewBorder(
		controls, nil, nil, nil,
		lt.logScroll,
	)

	lt.LoadExistingLogs()

	return logContainer
}

// AddLog adds a new log entry
func (lt *LogsTab) AddLog(message string) {
	timestamp := time.Now().Format("15:04:05")
	logEntry := fmt.Sprintf("[%s] %s", timestamp, message)

	lt.logBuffer = append(lt.logBuffer, logEntry)

	// Limit buffer size
	if len(lt.logBuffer) > lt.maxLogs {
		lt.logBuffer = lt.logBuffer[len(lt.logBuffer)-lt.maxLogs:]
	}

	lt.updateLogDisplay()

	if lt.autoScroll.Checked {
		lt.logScroll.ScrollToBottom()
	}
}

// ClearLogs clears all logs
func (lt *LogsTab) ClearLogs() {
	dialog.ShowConfirm("Confirm Clear",
		"Are you sure you want to clear all logs?",
		func(confirmed bool) {
			if confirmed {
				lt.logBuffer = []string{}
				lt.updateLogDisplay()
				lt.gui.updateStatus("Cleared all logs")
			}
		}, lt.gui.window)
}

// SaveLogs saves logs to a file
func (lt *LogsTab) SaveLogs() {
	if len(lt.logBuffer) == 0 {
		dialog.ShowInformation("No Data", "No logs to save", lt.gui.window)
		return
	}

	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}
		defer writer.Close()

		var lines []string
		lines = append(lines, "# LinkedIn Auto Crawler Logs")
		lines = append(lines, fmt.Sprintf("# Generated: %s", time.Now().Format("2006-01-02 15:04:05")))
		lines = append(lines, "")
		lines = append(lines, lt.logBuffer...)

		content := strings.Join(lines, "\n")

		_, err = writer.Write([]byte(content))
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to write file: %v", err), lt.gui.window)
			return
		}

		lt.gui.updateStatus(fmt.Sprintf("Saved %d log entries", len(lt.logBuffer)))
	}, lt.gui.window)
}

// LoadExistingLogs loads existing logs from crawler.log file
func (lt *LogsTab) LoadExistingLogs() {
	file, err := os.Open("crawler.log")
	if err != nil {
		lt.AddLog("No existing log file found")
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var tempBuffer []string
	for scanner.Scan() {
		tempBuffer = append(tempBuffer, scanner.Text())
		if len(tempBuffer) > lt.maxLogs {
			tempBuffer = tempBuffer[1:]
		}
	}

	for _, line := range tempBuffer {
		if strings.TrimSpace(line) != "" {
			lt.logBuffer = append(lt.logBuffer, line)
		}
	}
	lt.updateLogDisplay()
	if len(tempBuffer) > 0 {
		lt.AddLog(fmt.Sprintf("Loaded %d existing log entries", len(tempBuffer)))
	}
}

// updateLogDisplay updates the log display
func (lt *LogsTab) updateLogDisplay() {
	if len(lt.logBuffer) == 0 {
		lt.logText.ParseMarkdown("*No logs available*")
		return
	}

	// Filter logs based on level selection
	selectedLevel := lt.levelSelect.Selected
	var filteredLogs []string
	for _, log := range lt.logBuffer {
		if selectedLevel == "All" {
			filteredLogs = append(filteredLogs, log)
		} else {
			switch selectedLevel {
			case "Warning":
				if strings.Contains(strings.ToLower(log), "warning") || strings.Contains(log, "⚠️") {
					filteredLogs = append(filteredLogs, log)
				}
			case "Error":
				if strings.Contains(strings.ToLower(log), "error") || strings.Contains(log, "❌") {
					filteredLogs = append(filteredLogs, log)
				}
			case "Info":
				if !strings.Contains(strings.ToLower(log), "error") && !strings.Contains(strings.ToLower(log), "warning") {
					filteredLogs = append(filteredLogs, log)
				}
			}
		}
	}

	var displayText strings.Builder
	displayText.WriteString("```\n")

	if len(filteredLogs) == 0 {
		displayText.WriteString("No logs match the selected filter\n")
	} else {
		startIdx := 0
		if len(filteredLogs) > 100 {
			startIdx = len(filteredLogs) - 100
		}
		for i := startIdx; i < len(filteredLogs); i++ {
			displayText.WriteString(filteredLogs[i])
			displayText.WriteString("\n")
		}
	}
	displayText.WriteString("```")

	lt.logText.ParseMarkdown(displayText.String())
}

// Optional: GetLogs for export or other use
func (lt *LogsTab) GetLogs() []string {
	return lt.logBuffer
}
