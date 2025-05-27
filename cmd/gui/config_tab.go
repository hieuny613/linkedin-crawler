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

	// Set placeholders
	tab.maxConcurrency.SetPlaceHolder("50")
	tab.requestsPerSec.SetPlaceHolder("20.0")
	tab.requestTimeout.SetPlaceHolder("15s")
	tab.minTokens.SetPlaceHolder("10")
	tab.maxTokens.SetPlaceHolder("10")
	tab.sleepDuration.SetPlaceHolder("1m")

	// Initialize buttons
	tab.saveBtn = widget.NewButton("Save Configuration", tab.SaveConfig)
	tab.resetBtn = widget.NewButton("Reset to Defaults", tab.ResetConfig)

	// Style buttons
	tab.saveBtn.Importance = widget.HighImportance
	tab.resetBtn.Importance = widget.MediumImportance

	// Load config from preferences if available
	tab.loadFromPreferences()
	tab.updateFormFromConfig()

	return tab
}

// CreateContent creates the configuration tab content
func (ct *ConfigTab) CreateContent() fyne.CanvasObject {
	// Performance settings card
	perfForm := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Max Concurrency:", Widget: ct.maxConcurrency, HintText: "Maximum concurrent requests (1-100)"},
			{Text: "Requests Per Second:", Widget: ct.requestsPerSec, HintText: "Rate limit for requests (1.0-50.0)"},
			{Text: "Request Timeout:", Widget: ct.requestTimeout, HintText: "Timeout for each request (e.g., 15s)"},
		},
	}

	perfCard := widget.NewCard("Performance Settings",
		"Configure crawler performance and rate limiting", perfForm)

	// Token settings card
	tokenForm := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Minimum Tokens:", Widget: ct.minTokens, HintText: "Minimum tokens before refresh (5-20)"},
			{Text: "Maximum Tokens:", Widget: ct.maxTokens, HintText: "Maximum tokens to extract per batch (5-20)"},
			{Text: "Sleep Duration:", Widget: ct.sleepDuration, HintText: "Sleep before exit (e.g., 1m)"},
		},
	}

	tokenCard := widget.NewCard("Token Management",
		"Configure authentication token handling", tokenForm)

	// File paths card
	pathsInfo := widget.NewRichTextFromMarkdown(`
**File Paths:**
- **Emails File:** emails.txt
- **Tokens File:** tokens.txt (auto-generated)
- **Accounts File:** accounts.txt
- **Output File:** hit.txt
- **Log File:** crawler.log
- **Database:** emails.db
	`)

	pathsCard := widget.NewCard("File Configuration",
		"Default file paths used by the crawler", pathsInfo)

	// Buttons container
	buttonContainer := container.NewHBox(
		ct.saveBtn,
		ct.resetBtn,
		widget.NewButton("Load Default", ct.LoadConfig),
	)

	// Advanced settings
	advancedSettings := widget.NewAccordion(
		widget.NewAccordionItem("Advanced Settings", widget.NewRichTextFromMarkdown(`
**Rate Limiting Strategy:**
- Uses token bucket algorithm
- Automatic backoff on 429 errors
- Smart token rotation

**Concurrency Model:**
- Worker pool pattern
- Semaphore-based limiting
- Context-based cancellation

**Error Handling:**
- Automatic retry with exponential backoff
- Token validation and refresh
- Graceful degradation

**Memory Management:**
- Connection pooling
- Buffer size optimization
- Garbage collection friendly
		`)),
	)

	// Help section
	helpInfo := widget.NewRichTextFromMarkdown(`
### Configuration Guidelines

**Max Concurrency:** Higher values = faster processing but more resource usage  
**Requests Per Second:** Lower values = less likely to be rate limited  
**Request Timeout:** Higher values = more tolerance for slow responses  
**Min/Max Tokens:** Balance between token refresh frequency and batch size

### Recommended Settings

**Conservative:** Concurrency: 25, Rate: 10/s, Timeout: 20s  
**Balanced:** Concurrency: 50, Rate: 20/s, Timeout: 15s  
**Aggressive:** Concurrency: 75, Rate: 30/s, Timeout: 10s

### Performance Tips

- Start with conservative settings
- Monitor for 429 rate limit errors
- Adjust based on your network speed
- Use more accounts for higher rates
	`)

	helpCard := widget.NewCard("Help & Guidelines", "", helpInfo)

	// Layout
	leftColumn := container.NewVBox(
		perfCard,
		tokenCard,
		buttonContainer,
	)

	rightColumn := container.NewVBox(
		pathsCard,
		advancedSettings,
		helpCard,
	)

	content := container.NewHSplit(leftColumn, rightColumn)
	content.SetOffset(0.6) // 60% for left, 40% for right

	return container.NewScroll(content)
}

// LoadConfig loads configuration from default or saved settings
func (ct *ConfigTab) LoadConfig() {
	ct.config = config.DefaultConfig()
	ct.updateFormFromConfig()
	ct.gui.updateStatus("Configuration loaded")
}

// SaveConfig saves the current configuration
func (ct *ConfigTab) SaveConfig() {
	if err := ct.updateConfigFromForm(); err != nil {
		dialog.ShowError(err, ct.gui.window)
		return
	}

	// Save to app preferences
	ct.saveToPreferences()

	ct.gui.updateStatus("Configuration saved successfully")
	dialog.ShowInformation("Success", "Configuration saved successfully!", ct.gui.window)
}

// ResetConfig resets configuration to defaults
func (ct *ConfigTab) ResetConfig() {
	dialog.ShowConfirm("Reset Configuration",
		"Are you sure you want to reset all settings to defaults?",
		func(confirmed bool) {
			if confirmed {
				ct.config = config.DefaultConfig()
				ct.updateFormFromConfig()
				ct.gui.updateStatus("Configuration reset to defaults")
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
		return fmt.Errorf("max concurrency must be between 1 and 100")
	} else {
		ct.config.MaxConcurrency = val
	}

	// Parse RequestsPerSec
	if val, err := strconv.ParseFloat(ct.requestsPerSec.Text, 64); err != nil {
		return fmt.Errorf("invalid requests per second: %v", err)
	} else if val < 1.0 || val > 50.0 {
		return fmt.Errorf("requests per second must be between 1.0 and 50.0")
	} else {
		ct.config.RequestsPerSec = val
	}

	// Parse RequestTimeout
	if val, err := time.ParseDuration(ct.requestTimeout.Text); err != nil {
		return fmt.Errorf("invalid request timeout: %v", err)
	} else if val < time.Second || val > time.Minute {
		return fmt.Errorf("request timeout must be between 1s and 1m")
	} else {
		ct.config.RequestTimeout = val
	}

	// Parse MinTokens
	if val, err := strconv.Atoi(ct.minTokens.Text); err != nil {
		return fmt.Errorf("invalid min tokens: %v", err)
	} else if val < 1 || val > 50 {
		return fmt.Errorf("min tokens must be between 1 and 50")
	} else {
		ct.config.MinTokens = val
	}

	// Parse MaxTokens
	if val, err := strconv.Atoi(ct.maxTokens.Text); err != nil {
		return fmt.Errorf("invalid max tokens: %v", err)
	} else if val < 1 || val > 50 {
		return fmt.Errorf("max tokens must be between 1 and 50")
	} else {
		ct.config.MaxTokens = val
	}

	// Parse SleepDuration
	if val, err := time.ParseDuration(ct.sleepDuration.Text); err != nil {
		return fmt.Errorf("invalid sleep duration: %v", err)
	} else if val < 0 || val > 10*time.Minute {
		return fmt.Errorf("sleep duration must be between 0 and 10m")
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

	ct.updateFormFromConfig()
}
