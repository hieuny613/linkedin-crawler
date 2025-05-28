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

	gui := NewCrawlerGUI()

	// Bắt đầu goroutine xử lý mọi cập nhật UI qua channel updateUI
	go func() {
		for fn := range gui.updateUI {
			fn()
		}
	}()

	gui.setupUI()
	gui.loadSettings()
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
	// Mọi cập nhật UI đều gửi vào channel (kể cả khi gọi từ main goroutine)
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
		gui.updateUI <- func() { gui.updateStatus("Running...") }
		err = autoCrawler.Run()
		gui.crawlerMux.Lock()
		gui.isRunning = false
		gui.autoCrawler = nil
		gui.crawlerMux.Unlock()
		gui.updateUI <- func() { gui.updateStatus("Completed") }
		if err != nil {
			gui.updateUI <- func() { gui.updateStatus("Stopped with errors") }
		} else {
			gui.updateUI <- func() {
				gui.updateStatus("Completed successfully")
				gui.resultsTab.RefreshResults()
			}
		}
		gui.updateUI <- func() {
			if gui.window != nil {
				dialog.ShowInformation("Complete", "Crawling finished", gui.window)
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
}

func (gui *CrawlerGUI) updateStatus(status string) {
	if gui.statusBar != nil {
		gui.statusBar.SetText(status)
	}
}
