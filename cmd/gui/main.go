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

	// Add welcome message
	gui.logsTab.AddLog("🚀 LinkedIn Auto Crawler GUI started")
	gui.logsTab.AddLog("📋 Use the tabs to configure accounts, emails, and crawler settings")
	gui.logsTab.AddLog("▶️ Go to the Control tab to start crawling")

	// Run the application
	gui.window.ShowAndRun()
}

// NewCrawlerGUI creates a new GUI instance
func NewCrawlerGUI() *CrawlerGUI {
	a := app.NewWithID("com.linkedin.crawler.gui")
	a.SetIcon(theme.ComputerIcon())

	w := a.NewWindow("LinkedIn Auto Crawler - GUI Version")
	w.Resize(fyne.NewSize(1400, 900))
	w.SetMaster()
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

	return gui
}

// setupUI sets up the main user interface
func (gui *CrawlerGUI) setupUI() {
	// Create main tabs
	tabs := container.NewAppTabs(
		container.NewTabItem("⚙️ Configuration", gui.configTab.CreateContent()),
		container.NewTabItem("👥 Accounts", gui.accountsTab.CreateContent()),
		container.NewTabItem("📧 Emails", gui.emailsTab.CreateContent()),
		container.NewTabItem("🎮 Control", gui.controlTab.CreateContent()),
		container.NewTabItem("📊 Results", gui.resultsTab.CreateContent()),
		container.NewTabItem("📝 Logs", gui.logsTab.CreateContent()),
	)

	// Status bar
	gui.statusBar = widget.NewLabel("Ready - LinkedIn Auto Crawler GUI")
	gui.statusBar.Wrapping = fyne.TextWrapWord

	// Memory info
	memoryLabel := widget.NewLabel("")
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				memoryLabel.SetText(fmt.Sprintf("Memory: %d MB | Goroutines: %d",
					m.Alloc/1024/1024, runtime.NumGoroutine()))
			case <-gui.ctx.Done():
				return
			}
		}
	}()

	// Bottom status container with app info
	appInfo := widget.NewLabel("LinkedIn Auto Crawler v2.0 - GUI Edition")
	appInfo.TextStyle.Italic = true

	statusContainer := container.NewBorder(
		nil, nil,
		container.NewVBox(
			widget.NewLabel("Status:"),
			appInfo,
		),
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
				"Crawler is running. Do you want to stop it and exit?",
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
	// Load configuration
	gui.configTab.LoadConfig()

	// Load accounts
	gui.accountsTab.LoadAccounts()

	// Load emails
	gui.emailsTab.LoadEmails()

	// Update status
	gui.updateStatus("Settings loaded successfully")
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
		dialog.ShowError(fmt.Errorf("No Microsoft Teams accounts configured.\nPlease add accounts in the Accounts tab."), gui.window)
		return
	}

	if len(gui.emailsTab.emails) == 0 {
		dialog.ShowError(fmt.Errorf("No target emails configured.\nPlease add emails in the Emails tab."), gui.window)
		return
	}

	// Save current settings
	gui.saveSettings()

	// Show starting dialog
	progressDialog := dialog.NewProgressInfinite("Starting Crawler", "Initializing crawler components...", gui.window)
	progressDialog.Show()

	// Initialize crawler in goroutine
	go func() {
		defer progressDialog.Hide()

		cfg := gui.configTab.config
		autoCrawler, err := orchestrator.New(cfg)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to initialize crawler: %v", err), gui.window)
			return
		}

		gui.autoCrawler = autoCrawler
		gui.isRunning = true

		// Update UI
		gui.controlTab.OnCrawlerStarted()
		gui.updateStatus("Crawler started successfully")
		gui.logsTab.AddLog("🚀 Crawler initialization completed")
		gui.logsTab.AddLog(fmt.Sprintf("📊 Processing %d emails with %d accounts", len(gui.emailsTab.emails), len(gui.accountsTab.accounts)))

		// Start crawler
		err = autoCrawler.Run()

		// Cleanup after crawler finishes
		gui.crawlerMux.Lock()
		gui.isRunning = false
		gui.autoCrawler = nil
		gui.crawlerMux.Unlock()

		gui.controlTab.OnCrawlerStopped()

		if err != nil {
			gui.logsTab.AddLog(fmt.Sprintf("❌ Crawler finished with error: %v", err))
			gui.updateStatus("Crawler stopped with errors")
		} else {
			gui.logsTab.AddLog("✅ Crawler finished successfully")
			gui.updateStatus("Crawler completed successfully")

			// Auto-refresh results
			gui.resultsTab.RefreshResults()
		}

		// Show completion notification
		if gui.window != nil {
			dialog.ShowInformation("Crawler Finished",
				"The crawling process has completed. Check the Results tab for findings.",
				gui.window)
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

	gui.updateStatus("Stopping crawler...")
	gui.logsTab.AddLog("⏹️ Stop signal sent to crawler")
}

// cleanup performs cleanup when exiting
func (gui *CrawlerGUI) cleanup() {
	gui.cancel()

	if gui.controlTab.updateTicker != nil {
		gui.controlTab.updateTicker.Stop()
	}

	gui.saveSettings()
	gui.logsTab.AddLog("🔄 Application cleanup completed")
}

// updateStatus updates the status bar
func (gui *CrawlerGUI) updateStatus(status string) {
	if gui.statusLabel != nil {
		gui.statusLabel.SetText(status)
	} else {
		fmt.Println("[Status]", status) // hoặc ghi log ra terminal tạm thời
	}
}
