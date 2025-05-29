// Update cmd/gui/main.go - Fixed license integration

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

	"linkedin-crawler/internal/licensing"
	"linkedin-crawler/internal/orchestrator"
	"linkedin-crawler/internal/utils"
)

// CrawlerGUI represents the main GUI application with strict license control
type CrawlerGUI struct {
	app    fyne.App
	window fyne.Window

	autoCrawler *orchestrator.AutoCrawler
	crawlerMux  sync.RWMutex
	isRunning   bool

	configTab          *ConfigTab
	accountsTab        *AccountsTab
	emailsTab          *EmailsTab
	resultsTab         *ResultsTab
	statusBarContainer fyne.CanvasObject
	licenseTab         *LicenseTab

	statusBar *widget.Label

	ctx      context.Context
	cancel   context.CancelFunc
	updateUI chan func()

	// License integration
	licenseWrapper *licensing.LicensedCrawlerWrapper
	isLicenseValid bool
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

	// Initialize GUI
	gui := NewCrawlerGUI()

	// Single dispatcher
	go func() {
		for fn := range gui.updateUI {
			fyne.Do(func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Panic in UI update: %v\n%s", r, debug.Stack())
					}
				}()
				fn()
			})
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic recovered in main: %v\n%s", r, debug.Stack())
		}
		gui.cleanup()
	}()

	// Build UI first
	gui.setupUI()

	// STRICT LICENSE CHECK - Block app if no valid license
	gui.updateUI <- func() {
		gui.performLicenseCheck()
	}

	// Start the application
	gui.window.ShowAndRun()
}

func NewCrawlerGUI() *CrawlerGUI {
	a := app.NewWithID("com.linkedin.crawler.gui")
	a.SetIcon(theme.ComputerIcon())
	w := a.NewWindow("LinkedIn Auto Crawler - Licensed Version")
	w.Resize(fyne.NewSize(1200, 700))
	w.SetFixedSize(true)
	w.CenterOnScreen()
	ctx, cancel := context.WithCancel(context.Background())

	gui := &CrawlerGUI{
		app:            a,
		window:         w,
		ctx:            ctx,
		cancel:         cancel,
		isRunning:      false,
		updateUI:       make(chan func(), 100),
		licenseWrapper: licensing.NewLicensedCrawlerWrapper(),
		isLicenseValid: false, // Start with invalid license
	}

	// Initialize tabs
	gui.configTab = NewConfigTab(gui)
	gui.accountsTab = NewAccountsTab(gui)
	gui.emailsTab = NewEmailsTab(gui)
	gui.resultsTab = NewResultsTab(gui)
	gui.licenseTab = NewLicenseTab(gui)

	return gui
}

// performLicenseCheck performs initial license validation
func (gui *CrawlerGUI) performLicenseCheck() {
	// Try to validate existing license
	err := gui.licenseWrapper.ValidateAndStart()

	if err != nil {
		gui.isLicenseValid = false
		gui.showLicenseRequiredDialog()
	} else {
		gui.isLicenseValid = true
		gui.enableAppFeatures()
		gui.loadSettings()

		// Show license info
		info := gui.licenseWrapper.GetLicenseInfo()
		if userName, ok := info["user_name"].(string); ok {
			gui.updateStatus(fmt.Sprintf("Licensed to: %s - Ready", userName))
		}
	}
}

// showLicenseRequiredDialog shows license activation dialog
func (gui *CrawlerGUI) showLicenseRequiredDialog() {
	gui.disableAppFeatures()

	content := container.NewVBox(
		widget.NewRichTextFromMarkdown("## 🔐 License Required\n\nThis software requires a valid license to operate."),
		widget.NewSeparator(),
		widget.NewRichTextFromMarkdown(`**Available License Types:**
• **TRIAL**: 100 emails, 2 accounts, 30 days
• **PERSONAL**: 5,000 emails, 10 accounts, 1 year  
• **PRO**: Unlimited emails & accounts, 1 year

**Get Your License:**
1. Contact your software provider
2. Or generate a trial key using the License tab`),
	)

	d := dialog.NewCustom("License Required", "Go to License Tab", content, gui.window)
	d.SetOnClosed(func() {
		// Force user to License tab
		gui.selectLicenseTab()
	})
	d.Resize(fyne.NewSize(500, 350))
	d.Show()

	gui.updateStatus("❌ License required - Please activate your license")
}

// disableAppFeatures disables all tabs except License
func (gui *CrawlerGUI) disableAppFeatures() {
	// This will be implemented in setupUI to disable tabs
}

// enableAppFeatures enables all app features after valid license
func (gui *CrawlerGUI) enableAppFeatures() {
	gui.isLicenseValid = true
	// Auto-deduplicate hit.txt on startup only after license validation
	fmt.Println("🔄 Checking for duplicates in hit.txt...")
	utils.AutoDeduplicateOnStartup()

	// Validate hit.txt
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

// selectLicenseTab forces selection of License tab
func (gui *CrawlerGUI) selectLicenseTab() {
	// Implementation depends on your tab container setup
	// This should programmatically select the License tab
}

// setupUI builds the UI with license-aware tab control
func (gui *CrawlerGUI) setupUI() {
	// Create tabs - License tab is always enabled, others disabled initially
	tabs := container.NewAppTabs(
		container.NewTabItem("License", gui.licenseTab.CreateContent()),
		container.NewTabItem("Config", gui.createDisabledContent("Config", "License required to access configuration")),
		container.NewTabItem("Accounts", gui.createDisabledContent("Accounts", "License required to manage accounts")),
		container.NewTabItem("Emails", gui.createDisabledContent("Emails", "License required to process emails")),
		container.NewTabItem("Results", gui.createDisabledContent("Results", "License required to view results")),
	)

	// Store reference to tabs for later enabling
	gui.setupTabsReference(tabs)

	gui.statusBar = widget.NewLabel("Please activate your license")

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
		container.NewHBox(
			widget.NewButton("Check License", func() {
				gui.updateUI <- func() {
					gui.refreshLicense()
				}
			}),
			memoryLabel,
		),
		gui.statusBar,
	)

	content := container.NewBorder(
		nil, statusContainer, nil, nil,
		tabs,
	)
	gui.window.SetContent(content)

	// Close intercept with license check
	gui.window.SetCloseIntercept(func() {
		gui.updateUI <- func() {
			if gui.isRunning {
				dialog.ShowConfirm("Confirm Exit",
					"Crawler đang chạy. Dừng và thoát?",
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
		}
	})
}

// createDisabledContent creates disabled placeholder content for tabs
func (gui *CrawlerGUI) createDisabledContent(tabName, message string) fyne.CanvasObject {
	return container.NewCenter(
		container.NewVBox(
			widget.NewIcon(theme.ErrorIcon()),
			widget.NewLabel(fmt.Sprintf("%s Disabled", tabName)),
			widget.NewLabel(message),
			widget.NewSeparator(),
			widget.NewButton("Activate License", func() {
				gui.selectLicenseTab()
			}),
		),
	)
}

// setupTabsReference stores reference to tabs for later modification
func (gui *CrawlerGUI) setupTabsReference(tabs *container.AppTabs) {
	// Store tabs reference for enabling/disabling
	gui.window.SetContent(gui.window.Content()) // This is placeholder - you'll need proper implementation
}

// refreshLicense refreshes license status and updates UI accordingly
func (gui *CrawlerGUI) refreshLicense() {
	err := gui.licenseWrapper.ValidateAndStart()

	if err != nil {
		gui.isLicenseValid = false
		gui.updateStatus("❌ Invalid license - Please activate")
		gui.showLicenseError(err)
	} else {
		wasInvalid := !gui.isLicenseValid
		gui.isLicenseValid = true

		if wasInvalid {
			// License just became valid - enable features
			gui.enableAllTabs()
			gui.loadSettings()
		}

		info := gui.licenseWrapper.GetLicenseInfo()
		if userName, ok := info["user_name"].(string); ok {
			gui.updateStatus(fmt.Sprintf("✅ Licensed to: %s", userName))
		}
	}
}

// enableAllTabs enables all tabs after license validation

func (gui *CrawlerGUI) enableAllTabs() {
	// Tạo lại AppTabs với nội dung thật
	tabs := container.NewAppTabs(
		container.NewTabItem("License", gui.licenseTab.CreateContent()),
		container.NewTabItem("Config", gui.configTab.CreateContent()),
		container.NewTabItem("Accounts", gui.accountsTab.CreateContent()),
		container.NewTabItem("Emails", gui.emailsTab.CreateContent()),
		container.NewTabItem("Results", gui.resultsTab.CreateContent()),
	)
	// Reuse statusBarContainer
	content := container.NewBorder(nil, gui.statusBarContainer, nil, nil, tabs)
	gui.window.SetContent(content)

	dialog.ShowInformation("License Activated", "Tất cả tính năng đã được kích hoạt!", gui.window)
}

// showLicenseError shows license error
func (gui *CrawlerGUI) showLicenseError(err error) {
	dialog.ShowError(fmt.Errorf("License Error: %v", err), gui.window)
}

// loadSettings loads settings only if license is valid
func (gui *CrawlerGUI) loadSettings() {
	if !gui.isLicenseValid {
		return
	}

	gui.updateUI <- func() { gui.configTab.LoadConfig() }
	gui.updateUI <- func() { gui.accountsTab.LoadAccounts() }
	gui.updateUI <- func() { gui.emailsTab.LoadEmails() }
}

// startCrawler with strict license checks
func (gui *CrawlerGUI) startCrawler() {
	gui.crawlerMux.Lock()
	defer gui.crawlerMux.Unlock()

	if gui.isRunning {
		return
	}

	// STRICT LICENSE CHECK
	if !gui.isLicenseValid {
		gui.updateUI <- func() {
			dialog.ShowError(fmt.Errorf("Cannot start crawler: No valid license"), gui.window)
		}
		return
	}

	// Revalidate license before starting
	if err := gui.licenseWrapper.ValidateAndStart(); err != nil {
		gui.isLicenseValid = false
		gui.updateUI <- func() {
			dialog.ShowError(fmt.Errorf("License validation failed: %v", err), gui.window)
		}
		return
	}

	// Check feature access
	if !gui.licenseWrapper.CheckFeatureAccess(licensing.FeatureBasicCrawling) {
		gui.updateUI <- func() {
			dialog.ShowError(fmt.Errorf("Basic crawling feature not available in your license"), gui.window)
		}
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

	// Check usage limits
	emailCount := len(gui.emailsTab.emails)
	accountCount := len(gui.accountsTab.accounts)

	if err := gui.licenseWrapper.CheckCrawlingLimits(emailCount, accountCount); err != nil {
		gui.updateUI <- func() {
			dialog.ShowError(fmt.Errorf("Usage limits exceeded: %v", err), gui.window)
		}
		return
	}

	// Continue with crawler startup...
	gui.saveSettings()
	progressDialog := dialog.NewProgressInfinite("Starting...", "Initializing licensed crawler...", gui.window)
	gui.updateUI <- func() { progressDialog.Show() }

	go func() {
		defer func() { gui.updateUI <- func() { progressDialog.Hide() } }()

		cfg := gui.configTab.config
		autoCrawler, err := orchestrator.New(cfg)
		if err != nil {
			gui.updateUI <- func() {
				dialog.ShowError(fmt.Errorf("failed to initialize: %v", err), gui.window)
			}
			return
		}

		gui.autoCrawler = autoCrawler
		gui.isRunning = true

		err = autoCrawler.Run()

		gui.crawlerMux.Lock()
		gui.isRunning = false
		gui.autoCrawler = nil
		gui.crawlerMux.Unlock()

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

		gui.updateUI <- func() {
			if gui.window != nil {
				if err != nil {
					dialog.ShowError(fmt.Errorf("Crawling completed with errors: %v", err), gui.window)
				} else {
					dialog.ShowInformation("Complete", "Licensed crawling finished successfully", gui.window)
				}
			}
		}
	}()
}

// stopCrawler stops the crawler
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

// saveSettings saves settings only if license is valid
func (gui *CrawlerGUI) saveSettings() {
	if !gui.isLicenseValid {
		return
	}

	gui.updateUI <- func() { gui.configTab.SaveConfig() }
	gui.updateUI <- func() { gui.accountsTab.SaveAccounts() }
	gui.updateUI <- func() { gui.emailsTab.SaveEmails() }
}

// cleanup releases all resources
func (gui *CrawlerGUI) cleanup() {
	gui.cancel()
	gui.saveSettings()

	if gui.emailsTab != nil {
		gui.emailsTab.Cleanup()
	}
	if gui.accountsTab != nil {
		gui.accountsTab.Cleanup()
	}
	if gui.resultsTab != nil {
		gui.resultsTab.Cleanup()
	}
	if gui.licenseTab != nil {
		gui.licenseTab.Cleanup()
	}

	time.Sleep(100 * time.Millisecond)

	if gui.updateUI != nil {
		close(gui.updateUI)
		gui.updateUI = nil
	}

	runtime.GC()
}

// updateStatus sets the status bar text
func (gui *CrawlerGUI) updateStatus(status string) {
	if gui.statusBar != nil {
		gui.statusBar.SetText(status)
	}
}

// OnLicenseActivated callback when license is successfully activated
func (gui *CrawlerGUI) OnLicenseActivated() {
	gui.updateUI <- func() {
		gui.refreshLicense()
	}
}
