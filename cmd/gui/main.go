// cmd/gui/main.go - Enhanced với comprehensive license checking

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

// CrawlerGUI với enhanced license integration
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

	// Enhanced license integration
	licenseWrapper     *licensing.LicensedCrawlerWrapper
	isLicenseValid     bool
	licenseCheckTicker *time.Ticker

	// License usage tracking
	sessionStartTime   time.Time
	lastUsageCheck     time.Time
	usageCheckInterval time.Duration
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
		gui.performComprehensiveLicenseCheck()
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
		isLicenseValid: false,

		// License tracking
		sessionStartTime:   time.Now(),
		lastUsageCheck:     time.Now(),
		usageCheckInterval: 30 * time.Second, // Check usage every 30 seconds
	}

	// Initialize tabs
	gui.configTab = NewConfigTab(gui)
	gui.accountsTab = NewAccountsTab(gui)
	gui.emailsTab = NewEmailsTab(gui)
	gui.resultsTab = NewResultsTab(gui)
	gui.licenseTab = NewLicenseTab(gui)

	return gui
}

// performComprehensiveLicenseCheck thực hiện kiểm tra license toàn diện
func (gui *CrawlerGUI) performComprehensiveLicenseCheck() {
	log.Printf("🔒 Performing comprehensive license validation...")

	// Try to validate existing license
	err := gui.licenseWrapper.ValidateAndStart()

	if err != nil {
		log.Printf("❌ License validation failed: %v", err)
		gui.isLicenseValid = false
		gui.showLicenseRequiredDialog()
		gui.disableAppFeatures()
	} else {
		log.Printf("✅ License validation successful")
		gui.isLicenseValid = true
		gui.enableAppFeatures()
		gui.loadSettings()
		gui.startLicenseMonitoring()

		// Show license info
		info := gui.licenseWrapper.GetLicenseInfo()
		if userName, ok := info["user_name"].(string); ok {
			gui.updateStatus(fmt.Sprintf("Licensed to: %s - Ready", userName))
		}
	}
}

// startLicenseMonitoring bắt đầu theo dõi license và usage
func (gui *CrawlerGUI) startLicenseMonitoring() {
	if gui.licenseCheckTicker != nil {
		gui.licenseCheckTicker.Stop()
	}

	gui.licenseCheckTicker = time.NewTicker(gui.usageCheckInterval)
	go func() {
		defer func() {
			if gui.licenseCheckTicker != nil {
				gui.licenseCheckTicker.Stop()
			}
		}()

		for {
			select {
			case <-gui.licenseCheckTicker.C:
				gui.updateUI <- func() {
					gui.performPeriodicLicenseCheck()
				}
			case <-gui.ctx.Done():
				return
			}
		}
	}()
}

// performPeriodicLicenseCheck kiểm tra license định kỳ
func (gui *CrawlerGUI) performPeriodicLicenseCheck() {
	if !gui.isLicenseValid {
		return
	}

	// Update usage counters if crawler is running
	if gui.isRunning && gui.autoCrawler != nil {
		gui.updateUsageFromCrawler()
	}

	// Check license validity
	err := gui.licenseWrapper.ValidateAndStart()
	if err != nil {
		log.Printf("⚠️ License became invalid during runtime: %v", err)
		gui.isLicenseValid = false
		gui.handleLicenseBecameInvalid(err)
		return
	}

	// Check usage limits if crawler is running
	if gui.isRunning {
		gui.checkUsageLimitsDuringRuntime()
	}

	// Update status with license info
	gui.updateStatusWithLicenseInfo()
}

// updateUsageFromCrawler cập nhật usage từ crawler hiện tại
func (gui *CrawlerGUI) updateUsageFromCrawler() {
	gui.crawlerMux.RLock()
	autoCrawler := gui.autoCrawler
	gui.crawlerMux.RUnlock()

	if autoCrawler != nil {
		emailStorage, _, _ := autoCrawler.GetStorageServices()
		if emailStorage != nil {
			stats, err := emailStorage.GetEmailStats()
			if err == nil {
				processed := stats["success"] + stats["failed"]
				success := stats["success"]

				// Update license wrapper counters
				gui.licenseWrapper.UpdateUsageCounters(processed, success)
			}
		}
	}
}

// checkUsageLimitsDuringRuntime kiểm tra usage limits khi đang chạy
func (gui *CrawlerGUI) checkUsageLimitsDuringRuntime() {
	usageStats := gui.licenseWrapper.GetUsageStats()

	currentProcessed, ok1 := usageStats["current_processed_emails"].(int)
	maxEmails, ok2 := usageStats["max_emails"].(int)

	if ok1 && ok2 && maxEmails > 0 {
		// Check if approaching limit (90%)
		if float64(currentProcessed)/float64(maxEmails) >= 0.9 {
			remaining := maxEmails - currentProcessed
			if remaining <= 0 {
				gui.handleEmailLimitReached()
			} else if remaining <= 10 {
				gui.showApproachingLimitWarning(currentProcessed, maxEmails, remaining)
			}
		}
	}
}

// handleEmailLimitReached xử lý khi đạt giới hạn email
func (gui *CrawlerGUI) handleEmailLimitReached() {
	log.Printf("🚫 Email processing limit reached")

	if gui.isRunning {
		gui.stopCrawler()

		gui.updateUI <- func() {
			dialog.ShowInformation("License Limit Reached",
				"Email processing limit has been reached according to your license.\n\n"+
					"The crawler has been stopped. Please upgrade your license to process more emails.",
				gui.window)
		}
	}

	gui.updateStatus("❌ Email limit reached - Crawler stopped")
}

// showApproachingLimitWarning hiển thị cảnh báo khi gần đạt giới hạn
func (gui *CrawlerGUI) showApproachingLimitWarning(current, max, remaining int) {
	// Only show warning once per session to avoid spam
	if time.Since(gui.lastUsageCheck) > 5*time.Minute {
		gui.lastUsageCheck = time.Now()

		log.Printf("⚠️ Approaching email limit: %d/%d (remaining: %d)", current, max, remaining)
		gui.updateStatus(fmt.Sprintf("⚠️ Email limit: %d/%d (remaining: %d)", current, max, remaining))

		gui.updateUI <- func() {
			dialog.ShowInformation("Approaching License Limit",
				fmt.Sprintf("You are approaching your email processing limit.\n\n"+
					"Current: %d/%d emails processed\n"+
					"Remaining: %d emails\n\n"+
					"Consider upgrading your license for more capacity.", current, max, remaining),
				gui.window)
		}
	}
}

// handleLicenseBecameInvalid xử lý khi license bị invalid trong runtime
func (gui *CrawlerGUI) handleLicenseBecameInvalid(err error) {
	if gui.isRunning {
		gui.stopCrawler()
	}

	gui.disableAppFeatures()

	gui.updateUI <- func() {
		dialog.ShowError(fmt.Errorf("License became invalid: %v\n\nThe application will be restricted until a valid license is activated.", err), gui.window)
		gui.selectLicenseTab()
	}

	gui.updateStatus("❌ License invalid - Please reactivate")
}

// updateStatusWithLicenseInfo cập nhật status với thông tin license
func (gui *CrawlerGUI) updateStatusWithLicenseInfo() {
	usageStats := gui.licenseWrapper.GetUsageStats()

	if currentProcessed, ok := usageStats["current_processed_emails"].(int); ok {
		if maxEmails, ok := usageStats["max_emails"].(int); ok && maxEmails > 0 {
			remaining := maxEmails - currentProcessed
			gui.updateStatus(fmt.Sprintf("Licensed - Used: %d/%d emails (Remaining: %d)",
				currentProcessed, maxEmails, remaining))
		} else {
			gui.updateStatus(fmt.Sprintf("Licensed - Processed: %d emails (Unlimited)", currentProcessed))
		}
	}
}

// startCrawler với comprehensive license checks
func (gui *CrawlerGUI) startCrawler() {
	gui.crawlerMux.Lock()
	defer gui.crawlerMux.Unlock()

	if gui.isRunning {
		return
	}

	// COMPREHENSIVE LICENSE VALIDATION
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
			gui.selectLicenseTab()
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

	// COMPREHENSIVE USAGE LIMITS CHECK
	emailCount := len(gui.emailsTab.emails)
	accountCount := len(gui.accountsTab.accounts)

	if err := gui.licenseWrapper.CheckCrawlingLimits(emailCount, accountCount); err != nil {
		gui.updateUI <- func() {
			dialog.ShowError(fmt.Errorf("Usage limits exceeded: %v", err), gui.window)
		}
		return
	}

	// Reset usage counters for new crawling session
	gui.licenseWrapper.ResetUsageCounters()
	gui.sessionStartTime = time.Now()

	// Continue with crawler startup
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

		// CRITICAL: Inject license wrapper into batch processor
		batchProcessor := autoCrawler.GetBatchProcessor()
		if batchProcessor != nil {
			batchProcessor.SetLicenseWrapper(gui.licenseWrapper)
			log.Printf("✅ License wrapper injected into batch processor")
		}

		gui.autoCrawler = autoCrawler
		gui.isRunning = true

		// Start enhanced license monitoring
		if gui.licenseCheckTicker == nil {
			gui.startLicenseMonitoring()
		}

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
					// Show final usage stats
					gui.showFinalUsageStats()
				}
			}
		}
	}()
}

// showFinalUsageStats hiển thị thống kê usage cuối cùng
func (gui *CrawlerGUI) showFinalUsageStats() {
	usageStats := gui.licenseWrapper.GetUsageStats()

	currentProcessed, _ := usageStats["current_processed_emails"].(int)
	currentSuccess, _ := usageStats["current_success_emails"].(int)
	maxEmails, _ := usageStats["max_emails"].(int)
	sessionDuration, _ := usageStats["session_duration"].(string)

	var message string
	if maxEmails > 0 {
		remaining := maxEmails - currentProcessed
		message = fmt.Sprintf("Licensed crawling session completed!\n\n"+
			"📊 Session Statistics:\n"+
			"• Processed: %d emails\n"+
			"• Successful: %d emails\n"+
			"• License limit: %d/%d emails used\n"+
			"• Remaining: %d emails\n"+
			"• Session duration: %s\n\n"+
			"Thank you for using LinkedIn Crawler!",
			currentProcessed, currentSuccess, currentProcessed, maxEmails, remaining, sessionDuration)
	} else {
		message = fmt.Sprintf("Licensed crawling session completed!\n\n"+
			"📊 Session Statistics:\n"+
			"• Processed: %d emails\n"+
			"• Successful: %d emails\n"+
			"• License: Unlimited usage\n"+
			"• Session duration: %s\n\n"+
			"Thank you for using LinkedIn Crawler!",
			currentProcessed, currentSuccess, sessionDuration)
	}

	dialog.ShowInformation("Session Complete", message, gui.window)
}

// showLicenseRequiredDialog shows enhanced license activation dialog
func (gui *CrawlerGUI) showLicenseRequiredDialog() {
	gui.disableAppFeatures()

	content := container.NewVBox(
		widget.NewRichTextFromMarkdown("## 🔐 License Required\n\nThis software requires a valid license to operate."),
		widget.NewSeparator(),
		widget.NewRichTextFromMarkdown(`**Available License Types:**
• **TRIAL**: 100 emails, 2 accounts, 30 days - Perfect for testing
• **PERSONAL**: 5,000 emails, 10 accounts, 1 year - Great for individual use  
• **PRO**: Unlimited emails & accounts, 1 year - Best for business

**Get Your License:**
1. Contact your software provider for a license key
2. Or generate a trial key using the License tab
3. All licenses include full GUI access and basic crawling features

**Why License?**
• Ensures you get updates and support
• Helps fund continued development
• Provides usage tracking and limits`),
	)

	d := dialog.NewCustom("License Required", "Go to License Tab", content, gui.window)
	d.SetOnClosed(func() {
		// Force user to License tab
		gui.selectLicenseTab()
	})
	d.Resize(fyne.NewSize(550, 400))
	d.Show()

	gui.updateStatus("❌ License required - Please activate your license")
}

// disableAppFeatures disables all tabs except License
func (gui *CrawlerGUI) disableAppFeatures() {
	// This will be implemented in setupUI to disable tabs
	log.Printf("🚫 Disabling app features due to invalid license")
}

// enableAppFeatures enables all app features after valid license
func (gui *CrawlerGUI) enableAppFeatures() {
	gui.isLicenseValid = true
	log.Printf("✅ Enabling all app features - license is valid")

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
	// Implementation will depend on your tab container setup
	log.Printf("📋 Directing user to License tab")
}

// OnLicenseActivated callback when license is successfully activated
func (gui *CrawlerGUI) OnLicenseActivated() {
	gui.updateUI <- func() {
		gui.performComprehensiveLicenseCheck()
	}
}

// cleanup releases all resources including license monitoring
func (gui *CrawlerGUI) cleanup() {
	gui.cancel()

	// Stop license monitoring
	if gui.licenseCheckTicker != nil {
		gui.licenseCheckTicker.Stop()
		gui.licenseCheckTicker = nil
	}

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

// Rest of the existing methods remain the same...
func (gui *CrawlerGUI) setupUI() {
	// Existing setupUI implementation...
}

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

func (gui *CrawlerGUI) saveSettings() {
	if !gui.isLicenseValid {
		return
	}
	gui.updateUI <- func() { gui.configTab.SaveConfig() }
	gui.updateUI <- func() { gui.accountsTab.SaveAccounts() }
	gui.updateUI <- func() { gui.emailsTab.SaveEmails() }
}

func (gui *CrawlerGUI) loadSettings() {
	if !gui.isLicenseValid {
		return
	}
	gui.updateUI <- func() { gui.configTab.LoadConfig() }
	gui.updateUI <- func() { gui.accountsTab.LoadAccounts() }
	gui.updateUI <- func() { gui.emailsTab.LoadEmails() }
}

func (gui *CrawlerGUI) updateStatus(status string) {
	if gui.statusBar != nil {
		gui.statusBar.SetText(status)
	}
}
