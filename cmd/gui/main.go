package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"linkedin-crawler/internal/orchestrator"
)

// CrawlerGUI represents the main GUI application
type CrawlerGUI struct {
	app    fyne.App
	window fyne.Window

	// Crawler instance
	autoCrawler *orchestrator.AutoCrawler
	crawlerMux  sync.RWMutex
	isRunning   bool

	// UI Components
	configTab   *ConfigTab
	accountsTab *AccountsTab
	emailsTab   *EmailsTab
	controlTab  *ControlTab
	resultsTab  *ResultsTab
	logsTab     *LogsTab

	// Status
	statusBar *widget.Label

	// Context for cancellation
	ctx         context.Context
	cancel      context.CancelFunc
	statusLabel *widget.Label
	updateUI    chan func()
}

func main() {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Create app data directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Printf("Warning: Could not get config directory: %v", err)
	} else {
		appDir := filepath.Join(configDir, "linkedin-crawler")
		os.MkdirAll(appDir, 0755)
	}

	// Create the GUI application
	gui := NewCrawlerGUI()

	// Setup and show the window
	gui.setupUI()
	gui.loadSettings()

	// Run the application
	gui.window.ShowAndRun()
}

// NewCrawlerGUI creates a new GUI instance
func NewCrawlerGUI() *CrawlerGUI {
	a := app.NewWithID("com.linkedin.crawler.gui")
	a.SetIcon(theme.ComputerIcon())

	w := a.NewWindow("LinkedIn Auto Crawler")
	w.Resize(fyne.NewSize(1000, 600)) // Smaller fixed size
	w.SetFixedSize(true)              // Fixed size
	w.CenterOnScreen()

	ctx, cancel := context.WithCancel(context.Background())

	gui := &CrawlerGUI{
		app:       a,
		window:    w,
		ctx:       ctx,
		cancel:    cancel,
		isRunning: false,
	}

	// Initialize tabs
	gui.configTab = NewConfigTab(gui)
	gui.accountsTab = NewAccountsTab(gui)
	gui.emailsTab = NewEmailsTab(gui)
	gui.controlTab = NewControlTab(gui)
	gui.resultsTab = NewResultsTab(gui)
	gui.logsTab = NewLogsTab(gui)
	gui.updateUI = make(chan func(), 100)

	return gui
}

// setupUI sets up the main user interface
func (gui *CrawlerGUI) setupUI() {
	// Create main tabs
	tabs := container.NewAppTabs(
		container.NewTabItem("Config", gui.configTab.CreateContent()),
		container.NewTabItem("Accounts", gui.accountsTab.CreateContent()),
		container.NewTabItem("Emails", gui.emailsTab.CreateContent()),
		container.NewTabItem("Control", gui.controlTab.CreateContent()),
		container.NewTabItem("Results", gui.resultsTab.CreateContent()),
		container.NewTabItem("Logs", gui.logsTab.CreateContent()),
	)

	// Compact status bar
	gui.statusBar = widget.NewLabel("Ready")

	// Memory info
	memoryLabel := widget.NewLabel("")
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				memoryLabel.SetText(fmt.Sprintf("%d MB", m.Alloc/1024/1024))
			case <-gui.ctx.Done():
				return
			}
		}
	}()

	// Simple status container
	statusContainer := container.NewBorder(
		nil, nil,
		widget.NewLabel("Status:"),
		memoryLabel,
		gui.statusBar,
	)

	// Main container
	content := container.NewBorder(
		nil, statusContainer, nil, nil,
		tabs,
	)

	gui.window.SetContent(content)

	// Window close handler
	gui.window.SetCloseIntercept(func() {
		if gui.isRunning {
			dialog.ShowConfirm("Confirm Exit",
				"Crawler is running. Stop and exit?",
				func(confirmed bool) {
					if confirmed {
						gui.stopCrawler()
						gui.cleanup()
						gui.app.Quit()
					}
				}, gui.window)
		} else {
			gui.cleanup()
			gui.app.Quit()
		}
	})
}

// loadSettings loads saved settings
func (gui *CrawlerGUI) loadSettings() {
	gui.configTab.LoadConfig()
	gui.accountsTab.LoadAccounts()
	gui.emailsTab.LoadEmails()
	gui.updateStatus("Ready")
}

// saveSettings saves current settings
func (gui *CrawlerGUI) saveSettings() {
	gui.configTab.SaveConfig()
	gui.accountsTab.SaveAccounts()
	gui.emailsTab.SaveEmails()
}

// startCrawler starts the crawling process
func (gui *CrawlerGUI) startCrawler() {
	gui.crawlerMux.Lock()
	defer gui.crawlerMux.Unlock()

	if gui.isRunning {
		return
	}

	// Validate configuration
	if len(gui.accountsTab.accounts) == 0 {
		dialog.ShowError(fmt.Errorf("No accounts configured"), gui.window)
		return
	}

	if len(gui.emailsTab.emails) == 0 {
		dialog.ShowError(fmt.Errorf("No emails configured"), gui.window)
		return
	}

	// Save current settings
	gui.saveSettings()

	// Show starting dialog
	progressDialog := dialog.NewProgressInfinite("Starting...", "Initializing...", gui.window)
	progressDialog.Show()

	// Initialize crawler in goroutine
	go func() {
		defer progressDialog.Hide()

		cfg := gui.configTab.config
		autoCrawler, err := orchestrator.New(cfg)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to initialize: %v", err), gui.window)
			return
		}

		gui.autoCrawler = autoCrawler
		gui.isRunning = true

		// Update UI
		gui.controlTab.OnCrawlerStarted()
		gui.updateStatus("Running...")

		// Start crawler
		err = autoCrawler.Run()

		// Cleanup after crawler finishes
		gui.crawlerMux.Lock()
		gui.isRunning = false
		gui.autoCrawler = nil
		gui.crawlerMux.Unlock()

		gui.controlTab.OnCrawlerStopped()

		if err != nil {
			gui.updateStatus("Stopped with errors")
		} else {
			gui.updateStatus("Completed successfully")
			gui.resultsTab.RefreshResults()
		}

		// Show completion notification
		if gui.window != nil {
			dialog.ShowInformation("Complete", "Crawling finished", gui.window)
		}
	}()
}

// stopCrawler stops the crawling process
func (gui *CrawlerGUI) stopCrawler() {
	gui.crawlerMux.Lock()
	defer gui.crawlerMux.Unlock()

	if !gui.isRunning || gui.autoCrawler == nil {
		return
	}

	// Signal shutdown
	shutdownRequested := gui.autoCrawler.GetShutdownRequested()
	if shutdownRequested != nil {
		*shutdownRequested = 1
	}

	gui.updateStatus("Stopping...")
}

// cleanup performs cleanup when exiting
func (gui *CrawlerGUI) cleanup() {
	gui.cancel()

	if gui.controlTab.updateTicker != nil {
		gui.controlTab.updateTicker.Stop()
	}

	gui.saveSettings()
}

// updateStatus updates the status bar
func (gui *CrawlerGUI) updateStatus(status string) {
	if gui.statusBar != nil {
		gui.statusBar.SetText(status)
	}
}
