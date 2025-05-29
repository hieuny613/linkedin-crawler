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
	"linkedin-crawler/internal/utils"
)

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

	gui := NewCrawlerGUI()

	// Bắt đầu goroutine xử lý mọi cập nhật UI qua channel updateUI
	go func() {
		for fn := range gui.updateUI {
			fn()
		}
	}()

	gui.setupUI()
	gui.loadSettings()

	// Kiểm tra file hit.txt và cảnh báo nếu có vấn đề (cũng update qua UI channel)
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

	gui.window.ShowAndRun()
}

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
	gui.configTab = NewConfigTab(gui)
	gui.accountsTab = NewAccountsTab(gui)
	gui.emailsTab = NewEmailsTab(gui)
	gui.resultsTab = NewResultsTab(gui)
	return gui
}

func (gui *CrawlerGUI) setupUI() {
	tabs := container.NewAppTabs(
		container.NewTabItem("Config", gui.configTab.CreateContent()),
		container.NewTabItem("Accounts", gui.accountsTab.CreateContent()),
		container.NewTabItem("Emails", gui.emailsTab.CreateContent()),
		container.NewTabItem("Results", gui.resultsTab.CreateContent()),
	)

	gui.statusBar = widget.NewLabel("Ready")

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

func (gui *CrawlerGUI) loadSettings() {
	gui.updateUI <- func() { gui.configTab.LoadConfig() }
	gui.updateUI <- func() { gui.accountsTab.LoadAccounts() }
	gui.updateUI <- func() { gui.emailsTab.LoadEmails() }
	gui.updateUI <- func() { gui.updateStatus("Ready") }
}

func (gui *CrawlerGUI) saveSettings() {
	gui.updateUI <- func() { gui.configTab.SaveConfig() }
	gui.updateUI <- func() { gui.accountsTab.SaveAccounts() }
	gui.updateUI <- func() { gui.emailsTab.SaveEmails() }
}

func (gui *CrawlerGUI) startCrawler() {
	gui.crawlerMux.Lock()
	defer gui.crawlerMux.Unlock()
	if gui.isRunning {
		return
	}
	if len(gui.accountsTab.accounts) == 0 {
		gui.updateUI <- func() {
			dialog.ShowError(fmt.Errorf("No accounts configured"), gui.window)
		}
		return
	}
	if len(gui.emailsTab.emails) == 0 {
		gui.updateUI <- func() {
			dialog.ShowError(fmt.Errorf("No emails configured"), gui.window)
		}
		return
	}
	gui.saveSettings()
	progressDialog := dialog.NewProgressInfinite("Starting...", "Initializing...", gui.window)
	gui.updateUI <- func() { progressDialog.Show() }
	go func() {
		defer func() {
			gui.updateUI <- func() { progressDialog.Hide() }
		}()
		cfg := gui.configTab.config
		autoCrawler, err := orchestrator.New(cfg)
		if err != nil {
			gui.updateUI <- func() {
				dialog.ShowError(fmt.Errorf("Failed to initialize: %v", err), gui.window)
			}
			return
		}
		gui.autoCrawler = autoCrawler
		gui.isRunning = true

		// Notify emailsTab that crawler started
		gui.updateUI <- func() {
			gui.updateStatus("Running...")
			if gui.emailsTab != nil {
				gui.emailsTab.OnCrawlerStarted()
			}
		}

		err = autoCrawler.Run()
		gui.crawlerMux.Lock()
		gui.isRunning = false
		gui.autoCrawler = nil
		gui.crawlerMux.Unlock()

		gui.updateUI <- func() {
			// Notify emailsTab that crawler stopped
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

func (gui *CrawlerGUI) stopCrawler() {
	gui.crawlerMux.Lock()
	defer gui.crawlerMux.Unlock()
	if !gui.isRunning || gui.autoCrawler == nil {
		return
	}
	shutdownRequested := gui.autoCrawler.GetShutdownRequested()
	if shutdownRequested != nil {
		*shutdownRequested = 1
	}
	gui.updateUI <- func() { gui.updateStatus("Stopping...") }
}

func (gui *CrawlerGUI) cleanup() {
	gui.cancel()
	gui.saveSettings()

	// Stop all refresh tickers
	if gui.emailsTab != nil && gui.emailsTab.statsRefreshTicker != nil {
		gui.emailsTab.statsRefreshTicker.Stop()
	}
	if gui.resultsTab != nil {
		gui.resultsTab.Cleanup()
	}
	// Không còn cleanup cho controlTab!
	if gui.accountsTab != nil && gui.accountsTab.tokenInfoTicker != nil {
		gui.accountsTab.tokenInfoTicker.Stop()
	}
}

func (gui *CrawlerGUI) updateStatus(status string) {
	if gui.statusBar != nil {
		gui.statusBar.SetText(status)
	}
}
