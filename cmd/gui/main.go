package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"linkedin-crawler/internal/orchestrator"
	"linkedin-crawler/internal/utils"
)

// CrawlerGUI represents the main GUI application
type CrawlerGUI struct {
	app    fyne.App
	window fyne.Window

	autoCrawler *orchestrator.AutoCrawler
	crawlerMux  sync.RWMutex
	isRunning   bool

	configTab   *ConfigTab
	accountsTab *AccountsTab
	emailsTab   *EmailsTab
	resultsTab  *ResultsTab

	statusBar *widget.Label

	ctx      context.Context
	cancel   context.CancelFunc
	updateUI chan func()
}

func main() {
	// Set log flags
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Create app data directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Printf("Warning: Could not get config directory: %v", err)
	} else {
		appDir := filepath.Join(configDir, "linkedin-crawler")
		os.MkdirAll(appDir, 0755)
	}

	// Auto-deduplicate hit.txt on startup
	fmt.Println("🔄 Checking for duplicates in hit.txt...")
	utils.AutoDeduplicateOnStartup()

	// Initialize GUI
	gui := NewCrawlerGUI()

	// Single dispatcher: mọi cập nhật UI phải chạy qua fyne.Do
	go func() {
		for fn := range gui.updateUI {
			// Schedule on Fyne main event loop
			fyne.Do(func() {
				// Recover panic within UI update
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Panic in UI update: %v\n%s", r, debug.Stack())
					}
				}()
				fn()
			})
		}
	}()

	// Ensure cleanup on panic or exit
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic recovered in main: %v\n%s", r, debug.Stack())
		}
		gui.cleanup()
	}()

	// Build UI and load settings
	gui.setupUI()
	gui.loadSettings()

	// Validate hit.txt and show dialog if needed
	gui.updateUI <- func() {
		if _, err := os.Stat("hit.txt"); err == nil {
			issues := utils.ValidateHitFile("hit.txt")
			if len(issues) > 1 || (len(issues) == 1 && issues[0] != "File validation passed - no issues found") {
				issuesText := "Hit.txt validation results:\n\n" + fmt.Sprintf("Found %d issues:\n", len(issues))
				for i, issue := range issues {
					if i < 10 {
						issuesText += fmt.Sprintf("• %s\n", issue)
					}
				}
				if len(issues) > 10 {
					issuesText += fmt.Sprintf("... and %d more issues\n", len(issues)-10)
				}
				issuesText += "\nRecommendation: Use 'Remove Duplicates' in Results tab"
				dialog.ShowInformation("Hit.txt Validation", issuesText, gui.window)
			}
		}
	}

	// Start the application
	gui.window.ShowAndRun()
}

// NewCrawlerGUI creates and returns the GUI instance
func NewCrawlerGUI() *CrawlerGUI {
	a := app.NewWithID("com.linkedin.crawler.gui")
	a.SetIcon(theme.ComputerIcon())
	w := a.NewWindow("LinkedIn Auto Crawler")
	w.Resize(fyne.NewSize(1000, 600))
	w.SetFixedSize(true)
	w.CenterOnScreen()
	ctx, cancel := context.WithCancel(context.Background())
	gui := &CrawlerGUI{
		app:       a,
		window:    w,
		ctx:       ctx,
		cancel:    cancel,
		isRunning: false,
		updateUI:  make(chan func(), 100),
	}
	// Initialize tabs
	gui.configTab = NewConfigTab(gui)
	gui.accountsTab = NewAccountsTab(gui)
	gui.emailsTab = NewEmailsTab(gui)
	gui.resultsTab = NewResultsTab(gui)
	return gui
}

// setupUI builds and configures the main window content
func (gui *CrawlerGUI) setupUI() {
	tabs := container.NewAppTabs(
		container.NewTabItem("Config", gui.configTab.CreateContent()),
		container.NewTabItem("Accounts", gui.accountsTab.CreateContent()),
		container.NewTabItem("Emails", gui.emailsTab.CreateContent()),
		container.NewTabItem("Results", gui.resultsTab.CreateContent()),
	)

	gui.statusBar = widget.NewLabel("Ready")

	// Memory usage label
	memoryLabel := widget.NewLabel("")
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				val := fmt.Sprintf("%d MB", m.Alloc/1024/1024)
				gui.updateUI <- func() {
					memoryLabel.SetText(val)
				}
			case <-gui.ctx.Done():
				return
			}
		}
	}()

	statusContainer := container.NewBorder(
		nil, nil,
		widget.NewLabel("Status:"),
		memoryLabel,
		gui.statusBar,
	)

	content := container.NewBorder(
		nil, statusContainer, nil, nil,
		tabs,
	)
	gui.window.SetContent(content)

	// Intercept close to handle running crawler
	gui.window.SetCloseIntercept(func() {
		// Always attempt to stop crawler and exit process
		if gui.isRunning {
			dialog.ShowConfirm("Confirm Exit",
				"Crawler is running. Stop and exit?",
				func(confirmed bool) {
					if confirmed {
						gui.stopCrawler()
						gui.cleanup()
						gui.app.Quit()
						os.Exit(0)
					}
				}, gui.window)
		} else {
			gui.cleanup()
			gui.app.Quit()
			os.Exit(0)
		}
	})
}

// loadSettings pushes initial tab data to the UI
func (gui *CrawlerGUI) loadSettings() {
	gui.updateUI <- func() { gui.configTab.LoadConfig() }
	gui.updateUI <- func() { gui.accountsTab.LoadAccounts() }
	gui.updateUI <- func() { gui.emailsTab.LoadEmails() }
	gui.updateUI <- func() { gui.updateStatus("Ready") }
}

// saveSettings persists current tab data to files
func (gui *CrawlerGUI) saveSettings() {
	gui.updateUI <- func() { gui.configTab.SaveConfig() }
	gui.updateUI <- func() { gui.accountsTab.SaveAccounts() }
	gui.updateUI <- func() { gui.emailsTab.SaveEmails() }
}

// startCrawler kicks off the background crawl process
func (gui *CrawlerGUI) startCrawler() {
	gui.crawlerMux.Lock()
	defer gui.crawlerMux.Unlock()
	if gui.isRunning {
		return
	}
	// Validate inputs
	if len(gui.accountsTab.accounts) == 0 {
		gui.updateUI <- func() {
			dialog.ShowError(fmt.Errorf("no accounts configured"), gui.window)
		}
		return
	}
	if len(gui.emailsTab.emails) == 0 {
		gui.updateUI <- func() {
			dialog.ShowError(fmt.Errorf("no emails configured"), gui.window)
		}
		return
	}

	// Save before starting
	gui.saveSettings()
	progressDialog := dialog.NewProgressInfinite("Starting...", "Initializing...", gui.window)
	gui.updateUI <- func() { progressDialog.Show() }

	go func() {
		// Ensure hide dialog
		defer func() { gui.updateUI <- func() { progressDialog.Hide() } }()

		cfg := gui.configTab.config
		autoCrawler, err := orchestrator.New(cfg)
		if err != nil {
			gui.updateUI <- func() {
				dialog.ShowError(fmt.Errorf("failed to initialize: %v", err), gui.window)
			}
			return
		}

		// Mark running
		gui.autoCrawler = autoCrawler
		gui.isRunning = true

		err = autoCrawler.Run()
		// Reset running state
		gui.crawlerMux.Lock()
		gui.isRunning = false
		gui.autoCrawler = nil
		gui.crawlerMux.Unlock()

		// Notify stop
		gui.updateUI <- func() {
			if gui.emailsTab != nil {
				gui.emailsTab.OnCrawlerStopped()
			}
			if err != nil {
				gui.updateStatus("Stopped with errors")
			} else {
				gui.updateStatus("Completed successfully")
				gui.resultsTab.RefreshResults()
			}
		}

		// Show final dialog
		gui.updateUI <- func() {
			if gui.window != nil {
				if err != nil {
					dialog.ShowError(fmt.Errorf("Crawling completed with errors: %v", err), gui.window)
				} else {
					dialog.ShowInformation("Complete", "Crawling finished successfully", gui.window)
				}
			}
		}
	}()
}

// stopCrawler signals the running crawl to stop
func (gui *CrawlerGUI) stopCrawler() {
	gui.crawlerMux.Lock()
	defer gui.crawlerMux.Unlock()
	if !gui.isRunning || gui.autoCrawler == nil {
		return
	}
	down := gui.autoCrawler.GetShutdownRequested()
	if down != nil {
		*down = 1
	}
	gui.updateUI <- func() { gui.updateStatus("Stopping...") }
}

// cleanup releases all resources before exit
func (gui *CrawlerGUI) cleanup() {
	// Cancel background context
	gui.cancel()
	// Save settings
	gui.saveSettings()

	// Clean sub-tabs
	if gui.emailsTab != nil {
		gui.emailsTab.Cleanup()
	}
	if gui.accountsTab != nil {
		gui.accountsTab.Cleanup()
	}
	if gui.resultsTab != nil {
		gui.resultsTab.Cleanup()
	}

	// Wait briefly for pending UI ops
	time.Sleep(100 * time.Millisecond)

	// Close UI dispatcher
	if gui.updateUI != nil {
		close(gui.updateUI)
		gui.updateUI = nil
	}

	// Force GC
	runtime.GC()
}

// updateStatus sets the status bar text
func (gui *CrawlerGUI) updateStatus(status string) {
	if gui.statusBar != nil {
		gui.statusBar.SetText(status)
	}
}

// handlePanic recovers and shows a dialog on panic
func (gui *CrawlerGUI) handlePanic(component string) {
	if r := recover(); r != nil {
		log.Printf("Panic in %s: %v\n%s", component, r, debug.Stack())
		gui.updateUI <- func() {
			dialog.ShowError(fmt.Errorf("Internal error in %s", component), gui.window)
		}
	}
}
