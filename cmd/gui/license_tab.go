// cmd/gui/license_tab.go - Fixed version with proper error handling

package main

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"linkedin-crawler/internal/licensing"
)

// LicenseTab handles license management with proper error handling
type LicenseTab struct {
	gui            *CrawlerGUI
	licenseWrapper *licensing.LicensedCrawlerWrapper

	// UI components
	statusCard      *widget.Card
	activationCard  *widget.Card
	licenseKeyEntry *widget.Entry
	activateBtn     *widget.Button
	removeBtn       *widget.Button
	refreshBtn      *widget.Button

	// Status display
	statusLabel   *widget.RichText
	userInfoLabel *widget.Label
	typeLabel     *widget.Label
	expiryLabel   *widget.Label
	limitsLabel   *widget.Label
	featuresLabel *widget.RichText

	// License info refresh ticker
	refreshTicker *time.Ticker
}

// NewLicenseTab creates a new license management tab
func NewLicenseTab(gui *CrawlerGUI) *LicenseTab {
	tab := &LicenseTab{
		gui:            gui,
		licenseWrapper: licensing.NewLicensedCrawlerWrapper(),
	}

	// Initialize UI components
	tab.setupUI()

	// Start auto-refresh
	tab.startAutoRefresh()

	return tab
}

// setupUI initializes all UI components
func (lt *LicenseTab) setupUI() {
	// License key entry with better placeholder
	lt.licenseKeyEntry = widget.NewEntry()
	lt.licenseKeyEntry.SetPlaceHolder("Enter license key: TYPE-USERNAME-EMAIL-EXPIRY-CHECKSUM")
	lt.licenseKeyEntry.MultiLine = false
	lt.licenseKeyEntry.Wrapping = fyne.TextWrapWord

	// Buttons
	lt.activateBtn = widget.NewButtonWithIcon("Activate License", theme.ConfirmIcon(), lt.ActivateLicense)
	lt.activateBtn.Importance = widget.HighImportance

	lt.removeBtn = widget.NewButtonWithIcon("Remove License", theme.DeleteIcon(), lt.RemoveLicense)
	lt.removeBtn.Importance = widget.DangerImportance

	lt.refreshBtn = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), lt.RefreshLicenseInfo)

	// Status components
	lt.statusLabel = widget.NewRichText()
	lt.userInfoLabel = widget.NewLabel("No license information")
	lt.typeLabel = widget.NewLabel("License Type: Unknown")
	lt.expiryLabel = widget.NewLabel("Expiry: Unknown")
	lt.limitsLabel = widget.NewLabel("Limits: Unknown")
	lt.featuresLabel = widget.NewRichText()

	// Update initial status
	lt.updateLicenseDisplay()
}

// CreateContent creates the license tab content
func (lt *LicenseTab) CreateContent() fyne.CanvasObject {
	// License activation section with examples
	exampleText := widget.NewRichTextFromMarkdown(`**License Key Examples:**
• TRIAL-JOHN-john@email.com-20241201-ABC123
• PERSONAL-JANE-jane@company.com-20251201-XYZ789
• PRO-COMPANY-admin@company.com-20251201-DEF456`)

	activationForm := container.NewVBox(
		widget.NewLabel("Enter License Key:"),
		lt.licenseKeyEntry,
		container.NewHBox(
			lt.activateBtn,
			lt.removeBtn,
		),
		widget.NewSeparator(),
		exampleText,
		widget.NewSeparator(),
		container.NewHBox(
			widget.NewButton("Generate Trial", lt.GenerateTrialKey),
			widget.NewButton("Help", lt.ShowHelp),
			widget.NewButton("Contact Support", lt.ContactSupport),
		),
	)

	lt.activationCard = widget.NewCard("License Activation", "", activationForm)

	// License status section
	statusContent := container.NewVBox(
		container.NewHBox(lt.refreshBtn),
		widget.NewSeparator(),
		lt.statusLabel,
		widget.NewSeparator(),
		lt.userInfoLabel,
		lt.typeLabel,
		lt.expiryLabel,
		lt.limitsLabel,
		widget.NewSeparator(),
		widget.NewLabel("Available Features:"),
		lt.featuresLabel,
	)

	lt.statusCard = widget.NewCard("License Status", "", statusContent)

	// Main layout
	content := container.NewVSplit(
		lt.activationCard,
		lt.statusCard,
	)
	content.SetOffset(0.4)

	return content
}

// ActivateLicense activates a license with improved error handling
func (lt *LicenseTab) ActivateLicense() {
	licenseKey := strings.TrimSpace(lt.licenseKeyEntry.Text)

	if licenseKey == "" {
		dialog.ShowError(fmt.Errorf("Please enter a license key"), lt.gui.window)
		return
	}

	// Validate key format first
	if !lt.isValidKeyFormat(licenseKey) {
		dialog.ShowError(fmt.Errorf("Invalid license key format.\n\nExpected format: TYPE-USERNAME-EMAIL-EXPIRY-CHECKSUM\nExample: PRO-JOHN-john@email.com-20241201-ABC123"), lt.gui.window)
		return
	}

	// Show progress
	progress := dialog.NewProgressInfinite("Activating", "Validating license key...", lt.gui.window)
	progress.Show()

	go func() {
		defer progress.Hide()

		// First validate the key
		lm := licensing.NewLicenseManager()
		info, validateErr := lm.ValidateLicenseKey(licenseKey)

		lt.gui.updateUI <- func() {
			if validateErr != nil {
				// Show detailed error message
				errorMsg := fmt.Sprintf("License validation failed:\n\n%v\n\nPlease check:\n• Key format is correct\n• Key has not expired\n• Key is not corrupted", validateErr)
				dialog.ShowError(fmt.Errorf(errorMsg), lt.gui.window)
				return
			}

			// Check if expired
			if time.Now().After(info.ExpiresAt) {
				errorMsg := fmt.Sprintf("License has expired on %s\n\nPlease contact your provider for a new license.", info.ExpiresAt.Format("2006-01-02"))
				dialog.ShowError(fmt.Errorf(errorMsg), lt.gui.window)
				return
			}

			// Save the license
			err := lt.licenseWrapper.ActivateLicense(licenseKey)
			if err != nil {
				errorMsg := fmt.Sprintf("Failed to save license:\n\n%v\n\nPlease try again or contact support.", err)
				dialog.ShowError(fmt.Errorf(errorMsg), lt.gui.window)
				return
			}

			// Success!
			lt.licenseKeyEntry.SetText("") // Clear the entry
			lt.updateLicenseDisplay()

			// Show success with license details
			successMsg := fmt.Sprintf("License activated successfully!\n\n"+
				"User: %s\n"+
				"Type: %s\n"+
				"Expires: Never (Lifetime)\n"+
				"Email Limit: %s\n"+
				"Account Limit: Unlimited",
				info.UserName,
				strings.ToUpper(string(info.Type)),
				func() string {
					if info.MaxEmails <= 0 {
						return "Unlimited"
					}
					return fmt.Sprintf("%d", info.MaxEmails)
				}())

			dialog.ShowInformation("License Activated", successMsg, lt.gui.window)
			lt.gui.updateStatus("✅ License activated successfully")

			// Notify main GUI that license was activated
			if lt.gui.OnLicenseActivated != nil {
				lt.gui.OnLicenseActivated()
			}
		}
	}()
}

// isValidKeyFormat checks if license key has valid format
func (lt *LicenseTab) isValidKeyFormat(key string) bool {
	// Remove spaces and convert to upper
	key = strings.ReplaceAll(key, " ", "")
	key = strings.ToUpper(key)

	// Check basic format: TYPE-USERNAME-EMAIL-EXPIRY-CHECKSUM
	parts := strings.Split(key, "-")
	if len(parts) < 5 {
		return false
	}

	// Check license type
	licenseType := strings.ToLower(parts[0])
	validTypes := []string{"trial", "personal", "pro"}
	found := false
	for _, validType := range validTypes {
		if licenseType == validType {
			found = true
			break
		}
	}

	return found
}

// RemoveLicense removes the current license with confirmation
func (lt *LicenseTab) RemoveLicense() {
	// Check if license exists first
	info := lt.licenseWrapper.GetLicenseInfo()
	if status, ok := info["status"].(string); !ok || status == "invalid" {
		dialog.ShowInformation("No License", "No license is currently installed.", lt.gui.window)
		return
	}

	userName, _ := info["user_name"].(string)
	licenseType, _ := info["type"].(string)

	confirmMsg := fmt.Sprintf("Remove license for:\n\nUser: %s\nType: %s\n\nThis will disable the application until a new license is activated.\n\nContinue?", userName, strings.ToUpper(licenseType))

	dialog.ShowConfirm("Remove License", confirmMsg,
		func(confirmed bool) {
			if confirmed {
				err := lt.licenseWrapper.RemoveLicense()
				if err != nil {
					dialog.ShowError(fmt.Errorf("Failed to remove license: %v", err), lt.gui.window)
				} else {
					lt.updateLicenseDisplay()
					dialog.ShowInformation("License Removed", "License has been removed successfully.\n\nThe application will now require license activation to function.", lt.gui.window)
					lt.gui.updateStatus("❌ License removed - Please activate")

					// Notify main GUI that license was removed
					lt.gui.isLicenseValid = false
				}
			}
		}, lt.gui.window)
}

// RefreshLicenseInfo refreshes the license information display
func (lt *LicenseTab) RefreshLicenseInfo() {
	lt.updateLicenseDisplay()
	lt.gui.updateStatus("License information refreshed")
}

// GenerateTrialKey generates a trial license key for testing
func (lt *LicenseTab) GenerateTrialKey() {
	// Create form for trial key generation
	userNameEntry := widget.NewEntry()
	userNameEntry.SetText("TRIAL")
	emailEntry := widget.NewEntry()
	emailEntry.SetText("trial@example.com")

	form := []*widget.FormItem{
		{Text: "User Name:", Widget: userNameEntry},
		{Text: "Email:", Widget: emailEntry},
	}

	dialog.ShowForm("Generate Trial License", "Generate", "Cancel", form,
		func(confirmed bool) {
			if !confirmed {
				return
			}

			userName := strings.TrimSpace(strings.ToUpper(userNameEntry.Text))
			userEmail := strings.TrimSpace(emailEntry.Text)

			if userName == "" {
				userName = "TRIAL"
			}
			if userEmail == "" {
				userEmail = "trial@example.com"
			}

			// Generate trial key (30 days)
			trialKey := licensing.GenerateLicenseKey(licensing.LicenseTypeTrial, userName, userEmail, 30)

			// Show the generated key with detailed info
			content := fmt.Sprintf(`**Your Trial License Key:**

%s

**License Details:**
• User: %s (%s)
• Type: TRIAL
• Valid for: 30 days
• Expires: %s
• Email limit: 100
• Account limit: 2
• Features: Basic crawling, GUI interface

**Instructions:**
1. Copy the license key above
2. Paste it in the license key field
3. Click "Activate License"`,
				trialKey, userName, userEmail,
				time.Now().AddDate(0, 0, 30).Format("2006-01-02"))

			// Create custom dialog with selectable text
			richText := widget.NewRichTextFromMarkdown(content)
			richText.Wrapping = fyne.TextWrapWord

			scroll := container.NewScroll(richText)
			scroll.SetMinSize(fyne.NewSize(500, 300))

			d := dialog.NewCustom("Trial License Generated", "Close", scroll, lt.gui.window)
			d.Resize(fyne.NewSize(600, 400))
			d.Show()

			// Auto-fill the entry
			lt.licenseKeyEntry.SetText(trialKey)
		}, lt.gui.window)
}

// ShowHelp shows comprehensive license help
func (lt *LicenseTab) ShowHelp() {
	helpContent := `# LinkedIn Crawler License System

## License Types

### 🆓 TRIAL License
- **Duration**: Lifetime (Never expires)
- **Email limit**: 100 emails
- **Account limit**: Unlimited
- **Features**: Basic crawling, GUI interface
- **Best for**: Small projects, testing

### 👤 PERSONAL License
- **Duration**: Lifetime (Never expires)
- **Email limit**: 5,000 emails
- **Account limit**: Unlimited
- **Features**: All trial features + bulk processing, export tools
- **Best for**: Individual users, medium projects

### 🏢 PRO License  
- **Duration**: Lifetime (Never expires)
- **Email limit**: Unlimited
- **Account limit**: Unlimited
- **Features**: All features + advanced crawling, priority support
- **Best for**: Businesses, large-scale operations

## License Key Format

**Format**: TYPE-USERNAME-EMAIL-EXPIRY-CHECKSUM

**Examples**:
- TRIAL-JOHN-john@email.com-20241201-ABC123
- PERSONAL-JANE-jane@company.com-20251201-XYZ789
- PRO-COMPANY-admin@company.com-20251201-DEF456

**Note**: All licenses are lifetime and never expire. The expiry date in the key is for format compatibility only.

## Key Benefits

✅ **Lifetime License** - Never expires, buy once use forever
✅ **Unlimited Accounts** - Use as many accounts as you need
✅ **Only Email Limits** - Simple pricing based on email volume
✅ **All Features Included** - No feature restrictions by time

## Email Limits Explained

- **TRIAL**: Perfect for testing with 100 emails
- **PERSONAL**: Great for individual use with 5,000 emails  
- **PRO**: Unlimited emails for business use

Once you reach your email limit, simply upgrade to the next tier. Your license never expires!

## Activation Steps

1. **Obtain License Key**: Get your lifetime license key
2. **Enter Key**: Paste the key in the license field above
3. **Activate**: Click "Activate License" button
4. **Enjoy**: Use forever without worrying about expiry dates!

## Support & Upgrades

- **Trial → Personal**: Contact support to upgrade
- **Personal → Pro**: Contact support to upgrade  
- **Technical Issues**: Email support with your license key
- **Questions**: We're here to help 24/7`

	// Create scrollable help dialog
	richText := widget.NewRichTextFromMarkdown(helpContent)
	richText.Wrapping = fyne.TextWrapWord

	scroll := container.NewScroll(richText)
	scroll.SetMinSize(fyne.NewSize(600, 400))

	helpDialog := dialog.NewCustom("License Help", "Close", scroll, lt.gui.window)
	helpDialog.Resize(fyne.NewSize(700, 500))
	helpDialog.Show()
}

// ContactSupport shows support contact information
func (lt *LicenseTab) ContactSupport() {
	// Get current license info for support context
	info := lt.licenseWrapper.GetLicenseInfo()

	var supportMsg string
	if status, ok := info["status"].(string); ok && status != "invalid" {
		userName, _ := info["user_name"].(string)
		licenseType, _ := info["type"].(string)
		supportMsg = fmt.Sprintf(`**License Support**

**Your License Information:**
• User: %s
• Type: %s
• Status: %s

**Need Help?**

**For Technical Support:**
• Email: support@your-company.com
• Include your license information above
• Describe the issue you're experiencing

**For License Purchase/Renewal:**
• Email: sales@your-company.com
• Visit: https://your-website.com/licensing

**For Trial Licenses:**
• Use the "Generate Trial" button above
• Trial licenses are valid for 30 days
• No registration required

**Response Time:**
• Technical issues: 24-48 hours
• License issues: Same day
• Sales inquiries: 24 hours`, userName, strings.ToUpper(licenseType), status)
	} else {
		supportMsg = `**License Support**

**No Active License Found**

**Need a License?**

**For License Purchase:**
• Email: sales@your-company.com
• Visit: https://your-website.com/licensing

**For Trial License:**
• Click "Generate Trial" button above
• Free 30-day trial available
• No registration required

**For Technical Support:**
• Email: support@your-company.com
• Include screenshot of any error messages

**Available License Types:**
• **TRIAL**: Free 30-day trial (100 emails, 2 accounts)
• **PERSONAL**: $99/year (5,000 emails, 10 accounts)
• **PRO**: $299/year (unlimited emails & accounts)

**Response Time:**
• License issues: Same day
• Technical issues: 24-48 hours
• Sales inquiries: 24 hours`
	}

	richText := widget.NewRichTextFromMarkdown(supportMsg)
	richText.Wrapping = fyne.TextWrapWord

	scroll := container.NewScroll(richText)
	scroll.SetMinSize(fyne.NewSize(500, 300))

	supportDialog := dialog.NewCustom("Contact Support", "Close", scroll, lt.gui.window)
	supportDialog.Resize(fyne.NewSize(600, 400))
	supportDialog.Show()
}

// updateLicenseDisplay updates the license information display
func (lt *LicenseTab) updateLicenseDisplay() {
	info := lt.licenseWrapper.GetLicenseInfo()

	// Update status
	status, ok := info["status"].(string)
	if !ok {
		status = "unknown"
	}

	var statusText string

	switch status {
	case "active":
		statusText = "## ✅ LICENSE ACTIVE\n\nYour license is valid and all features are available."
	case "expiring_soon":
		daysLeft, _ := info["days_left"].(int)
		statusText = fmt.Sprintf("## ⚠️ LICENSE EXPIRING SOON\n\nYour license expires in %d days. Please renew to continue using the software.", daysLeft)
	case "expired":
		statusText = "## ❌ LICENSE EXPIRED\n\nYour license has expired. Please renew to continue using the software."
	case "invalid":
		errorMsg, _ := info["error"].(string)
		statusText = fmt.Sprintf("## ❌ NO VALID LICENSE\n\n%s\n\nPlease activate a valid license to use the software.", errorMsg)
	default:
		statusText = "## ❓ LICENSE STATUS UNKNOWN\n\nUnable to determine license status. Please check your license."
	}

	lt.statusLabel.ParseMarkdown(statusText)

	// Update user info
	if userName, ok := info["user_name"].(string); ok && userName != "" {
		userEmail, _ := info["user_email"].(string)
		lt.userInfoLabel.SetText(fmt.Sprintf("👤 Licensed to: %s (%s)", userName, userEmail))
	} else {
		lt.userInfoLabel.SetText("👤 User: No active license")
	}

	// Update license type
	if licenseType, ok := info["type"].(string); ok && licenseType != "" {
		lt.typeLabel.SetText(fmt.Sprintf("📄 License Type: %s", strings.ToUpper(licenseType)))
	} else {
		lt.typeLabel.SetText("📄 License Type: None")
	}

	// Update expiry - show as never expires
	lt.expiryLabel.SetText("📅 Expires: Never (Lifetime License)")

	// Update limits - show only email limits
	if maxEmails, ok := info["max_emails"].(int); ok {
		var emailLimit string

		if maxEmails <= 0 {
			emailLimit = "Unlimited"
		} else {
			emailLimit = fmt.Sprintf("%d", maxEmails)
		}

		lt.limitsLabel.SetText(fmt.Sprintf("📊 Email Limit: %s | Accounts: Unlimited", emailLimit))
	} else {
		lt.limitsLabel.SetText("📊 Limits: Not available")
	}

	// Update features
	if features, ok := info["features"].([]interface{}); ok && len(features) > 0 {
		var featureList []string
		for _, f := range features {
			if feature, ok := f.(string); ok {
				// Make feature names more readable
				readableName := strings.ReplaceAll(feature, "_", " ")
				readableName = strings.Title(readableName)
				featureList = append(featureList, "• "+readableName)
			}
		}

		if len(featureList) > 0 {
			featuresText := strings.Join(featureList, "\n")
			lt.featuresLabel.ParseMarkdown(featuresText)
		} else {
			lt.featuresLabel.ParseMarkdown("• No features available")
		}
	} else {
		lt.featuresLabel.ParseMarkdown("*No license active - please activate a license to view available features*")
	}

	// Update button states
	hasValidLicense := status == "active" || status == "expiring_soon"
	if hasValidLicense {
		lt.removeBtn.Enable()
	} else {
		lt.removeBtn.Disable()
	}
}

// startAutoRefresh starts automatic license info refresh
func (lt *LicenseTab) startAutoRefresh() {
	if lt.refreshTicker != nil {
		lt.refreshTicker.Stop()
	}

	lt.refreshTicker = time.NewTicker(30 * time.Second)
	go func() {
		defer func() {
			if lt.refreshTicker != nil {
				lt.refreshTicker.Stop()
			}
		}()

		for {
			select {
			case <-lt.refreshTicker.C:
				lt.gui.updateUI <- func() {
					lt.updateLicenseDisplay()
				}
			case <-lt.gui.ctx.Done():
				return
			}
		}
	}()
}

// ValidateLicenseForApp validates license before app operations
func (lt *LicenseTab) ValidateLicenseForApp() error {
	return lt.licenseWrapper.ValidateAndStart()
}

// CheckCrawlingLimits checks if crawling limits are within license
func (lt *LicenseTab) CheckCrawlingLimits(emailCount, accountCount int) error {
	return lt.licenseWrapper.CheckCrawlingLimits(emailCount, accountCount)
}

// CheckFeatureAccess checks if a feature is available
func (lt *LicenseTab) CheckFeatureAccess(feature string) bool {
	return lt.licenseWrapper.CheckFeatureAccess(feature)
}

// Cleanup stops the refresh ticker
func (lt *LicenseTab) Cleanup() {
	if lt.refreshTicker != nil {
		lt.refreshTicker.Stop()
		lt.refreshTicker = nil
	}
}
