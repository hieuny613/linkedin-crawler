package main

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"linkedin-crawler/internal/config"
)

// NewConfigTab creates a new configuration tab
func NewConfigTab(gui *CrawlerGUI) *ConfigTab {
	tab := &ConfigTab{
		gui:    gui,
		config: config.DefaultConfig(),
	}

	// Initialize form fields
	tab.maxConcurrency = widget.NewEntry()
	tab.requestsPerSec = widget.NewEntry()
	tab.requestTimeout = widget.NewEntry()
	tab.minTokens = widget.NewEntry()
	tab.maxTokens = widget.NewEntry()
	tab.sleepDuration = widget.NewEntry()

	// Set values
	tab.maxConcurrency.SetText("50")
	tab.requestsPerSec.SetText("20.0")
	tab.requestTimeout.SetText("15s")
	tab.minTokens.SetText("10")
	tab.maxTokens.SetText("10")
	tab.sleepDuration.SetText("30s")

	// Initialize buttons
	tab.saveBtn = widget.NewButton("Save", tab.SaveConfig)
	tab.resetBtn = widget.NewButton("Reset", tab.ResetConfig)

	// Style buttons
	tab.saveBtn.Importance = widget.HighImportance

	// Load config from preferences
	tab.loadFromPreferences()
	tab.updateFormFromConfig()

	return tab
}

// CreateContent creates the configuration tab content
func (ct *ConfigTab) CreateContent() fyne.CanvasObject {
	// Performance settings
	perfForm := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Max Concurrency:", Widget: ct.maxConcurrency},
			{Text: "Requests/Sec:", Widget: ct.requestsPerSec},
			{Text: "Request Timeout:", Widget: ct.requestTimeout},
		},
	}

	// Token settings
	tokenForm := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Min Tokens:", Widget: ct.minTokens},
			{Text: "Max Tokens:", Widget: ct.maxTokens},
			{Text: "Sleep Duration:", Widget: ct.sleepDuration},
		},
	}

	// Buttons
	buttonContainer := container.NewHBox(
		ct.saveBtn,
		ct.resetBtn,
	)

	// Recommendations
	recInfo := widget.NewRichTextFromMarkdown(`**Recommended Settings:**
- Conservative: Concurrency 25, Rate 10/s
- Balanced: Concurrency 50, Rate 20/s  
- Aggressive: Concurrency 75, Rate 30/s`)

	// Layout in two columns
	leftColumn := container.NewVBox(
		widget.NewCard("Performance", "", perfForm),
		buttonContainer,
	)

	rightColumn := container.NewVBox(
		widget.NewCard("Token Management", "", tokenForm),
		widget.NewCard("Tips", "", recInfo),
	)

	return container.NewHSplit(leftColumn, rightColumn)
}

// LoadConfig loads configuration
func (ct *ConfigTab) LoadConfig() {
	ct.config = config.DefaultConfig()
	ct.updateFormFromConfig()
	ct.gui.updateStatus("Config loaded")
}

// SaveConfig saves the current configuration
func (ct *ConfigTab) SaveConfig() {
	if err := ct.updateConfigFromForm(); err != nil {
		dialog.ShowError(err, ct.gui.window)
		return
	}

	ct.saveToPreferences()
	ct.gui.updateStatus("Config saved")
}

// ResetConfig resets configuration to defaults
func (ct *ConfigTab) ResetConfig() {
	dialog.ShowConfirm("Reset Configuration",
		"Reset all settings to defaults?",
		func(confirmed bool) {
			if confirmed {
				ct.config = config.DefaultConfig()
				ct.updateFormFromConfig()
				ct.gui.updateStatus("Config reset")
			}
		}, ct.gui.window)
}

// updateFormFromConfig updates form fields from config
func (ct *ConfigTab) updateFormFromConfig() {
	ct.maxConcurrency.SetText(fmt.Sprintf("%d", ct.config.MaxConcurrency))
	ct.requestsPerSec.SetText(fmt.Sprintf("%.1f", ct.config.RequestsPerSec))
	ct.requestTimeout.SetText(ct.config.RequestTimeout.String())
	ct.minTokens.SetText(fmt.Sprintf("%d", ct.config.MinTokens))
	ct.maxTokens.SetText(fmt.Sprintf("%d", ct.config.MaxTokens))
	ct.sleepDuration.SetText(ct.config.SleepDuration.String())
}

// updateConfigFromForm updates config from form fields
func (ct *ConfigTab) updateConfigFromForm() error {
	// Parse MaxConcurrency
	if val, err := strconv.ParseInt(ct.maxConcurrency.Text, 10, 64); err != nil {
		return fmt.Errorf("invalid max concurrency: %v", err)
	} else if val < 1 || val > 100 {
		return fmt.Errorf("max concurrency must be 1-100")
	} else {
		ct.config.MaxConcurrency = val
	}

	// Parse RequestsPerSec
	if val, err := strconv.ParseFloat(ct.requestsPerSec.Text, 64); err != nil {
		return fmt.Errorf("invalid requests per second: %v", err)
	} else if val < 1.0 || val > 50.0 {
		return fmt.Errorf("requests per second must be 1.0-50.0")
	} else {
		ct.config.RequestsPerSec = val
	}

	// Parse RequestTimeout
	if val, err := time.ParseDuration(ct.requestTimeout.Text); err != nil {
		return fmt.Errorf("invalid request timeout: %v", err)
	} else {
		ct.config.RequestTimeout = val
	}

	// Parse MinTokens
	if val, err := strconv.Atoi(ct.minTokens.Text); err != nil {
		return fmt.Errorf("invalid min tokens: %v", err)
	} else if val < 1 || val > 50 {
		return fmt.Errorf("min tokens must be 1-50")
	} else {
		ct.config.MinTokens = val
	}

	// Parse MaxTokens
	if val, err := strconv.Atoi(ct.maxTokens.Text); err != nil {
		return fmt.Errorf("invalid max tokens: %v", err)
	} else if val < 1 || val > 50 {
		return fmt.Errorf("max tokens must be 1-50")
	} else {
		ct.config.MaxTokens = val
	}

	// Parse SleepDuration
	if val, err := time.ParseDuration(ct.sleepDuration.Text); err != nil {
		return fmt.Errorf("invalid sleep duration: %v", err)
	} else {
		ct.config.SleepDuration = val
	}

	return nil
}

// saveToPreferences saves config to app preferences
func (ct *ConfigTab) saveToPreferences() {
	prefs := ct.gui.app.Preferences()

	prefs.SetInt("max_concurrency", int(ct.config.MaxConcurrency))
	prefs.SetFloat("requests_per_sec", ct.config.RequestsPerSec)
	prefs.SetString("request_timeout", ct.config.RequestTimeout.String())
	prefs.SetInt("min_tokens", ct.config.MinTokens)
	prefs.SetInt("max_tokens", ct.config.MaxTokens)
	prefs.SetString("sleep_duration", ct.config.SleepDuration.String())
}

// loadFromPreferences loads config from app preferences
func (ct *ConfigTab) loadFromPreferences() {
	prefs := ct.gui.app.Preferences()

	if val := prefs.IntWithFallback("max_concurrency", int(ct.config.MaxConcurrency)); val > 0 {
		ct.config.MaxConcurrency = int64(val)
	}

	if val := prefs.FloatWithFallback("requests_per_sec", ct.config.RequestsPerSec); val > 0 {
		ct.config.RequestsPerSec = val
	}

	if val := prefs.StringWithFallback("request_timeout", ct.config.RequestTimeout.String()); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			ct.config.RequestTimeout = duration
		}
	}

	if val := prefs.IntWithFallback("min_tokens", ct.config.MinTokens); val > 0 {
		ct.config.MinTokens = val
	}

	if val := prefs.IntWithFallback("max_tokens", ct.config.MaxTokens); val > 0 {
		ct.config.MaxTokens = val
	}

	if val := prefs.StringWithFallback("sleep_duration", ct.config.SleepDuration.String()); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			ct.config.SleepDuration = duration
		}
	}
}
