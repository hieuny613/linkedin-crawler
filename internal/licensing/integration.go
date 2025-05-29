// internal/licensing/integration.go - Enhanced với real-time checking

package licensing

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// LicensedCrawlerWrapper với enhanced checking
type LicensedCrawlerWrapper struct {
	licenseManager *LicenseManager

	// Real-time tracking
	currentProcessedEmails int
	currentSuccessEmails   int
	startTime              time.Time
}

// NewLicensedCrawlerWrapper creates enhanced wrapper
func NewLicensedCrawlerWrapper() *LicensedCrawlerWrapper {
	return &LicensedCrawlerWrapper{
		licenseManager:         NewLicenseManager(),
		currentProcessedEmails: 0,
		currentSuccessEmails:   0,
		startTime:              time.Now(),
	}
}

// ValidateAndStart validates license với detailed checking
func (lcw *LicensedCrawlerWrapper) ValidateAndStart() error {
	info, err := lcw.licenseManager.LoadLicense()
	if err != nil {
		return lcw.handleLicenseError(err)
	}

	if !info.IsValid {
		return fmt.Errorf("license is not valid or has expired")
	}

	// Enhanced validation
	if time.Now().After(info.ExpiresAt) {
		return fmt.Errorf("license has expired on %s", info.ExpiresAt.Format("2006-01-02"))
	}

	lcw.showLicenseInfo(info)

	if !lcw.licenseManager.CheckFeature(FeatureGUIInterface) {
		log.Printf("⚠️ GUI feature not available in your license")
		return fmt.Errorf("GUI interface not available in your license type")
	}

	return nil
}

// CheckCrawlingLimits với enhanced checking
func (lcw *LicensedCrawlerWrapper) CheckCrawlingLimits(emailCount, accountCount int) error {
	maxEmails, maxAccounts, err := lcw.licenseManager.GetUsageLimits()
	if err != nil {
		return fmt.Errorf("license validation failed: %w", err)
	}

	// Check account limits
	if maxAccounts > 0 && accountCount > maxAccounts {
		return fmt.Errorf("account limit exceeded: %d/%d accounts (upgrade license for more)", accountCount, maxAccounts)
	}

	// Enhanced email limit checking
	if maxEmails > 0 {
		// Check current processed emails + new emails
		totalWillProcess := lcw.currentProcessedEmails + emailCount

		if totalWillProcess > maxEmails {
			return fmt.Errorf("email limit will be exceeded: %d + %d = %d > %d (upgrade license for more emails)",
				lcw.currentProcessedEmails, emailCount, totalWillProcess, maxEmails)
		}

		// Warning when approaching limit
		if float64(totalWillProcess)/float64(maxEmails) > 0.9 {
			log.Printf("⚠️ WARNING: Approaching email limit - %d/%d emails (%.1f%%)",
				totalWillProcess, maxEmails, float64(totalWillProcess)*100/float64(maxEmails))
		}
	}

	return nil
}

// CheckRealTimeLimits kiểm tra limits trong quá trình crawling
func (lcw *LicensedCrawlerWrapper) CheckRealTimeLimits(currentProcessed, currentSuccess int) error {
	maxEmails, _, err := lcw.licenseManager.GetUsageLimits()
	if err != nil {
		return fmt.Errorf("license validation failed: %w", err)
	}

	// Update internal counters
	lcw.currentProcessedEmails = currentProcessed
	lcw.currentSuccessEmails = currentSuccess

	// Check processed email limits
	if maxEmails > 0 {
		if currentProcessed >= maxEmails {
			return fmt.Errorf("email processing limit reached: %d/%d emails processed", currentProcessed, maxEmails)
		}

		// Alternative: Check success emails instead of processed
		// if currentSuccess >= maxEmails {
		//     return fmt.Errorf("successful email limit reached: %d/%d emails", currentSuccess, maxEmails)
		// }
	}

	return nil
}

// GetUsageStats returns current usage statistics
func (lcw *LicensedCrawlerWrapper) GetUsageStats() map[string]interface{} {
	maxEmails, maxAccounts, err := lcw.licenseManager.GetUsageLimits()
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	stats := map[string]interface{}{
		"current_processed_emails": lcw.currentProcessedEmails,
		"current_success_emails":   lcw.currentSuccessEmails,
		"max_emails":               maxEmails,
		"max_accounts":             maxAccounts,
		"session_duration":         time.Since(lcw.startTime).String(),
	}

	// Calculate percentages
	if maxEmails > 0 {
		stats["email_usage_percent"] = float64(lcw.currentProcessedEmails) * 100 / float64(maxEmails)
		stats["remaining_emails"] = maxEmails - lcw.currentProcessedEmails
	} else {
		stats["email_usage_percent"] = 0.0
		stats["remaining_emails"] = -1 // Unlimited
	}

	return stats
}

// ResetUsageCounters resets internal usage counters (for new session)
func (lcw *LicensedCrawlerWrapper) ResetUsageCounters() {
	lcw.currentProcessedEmails = 0
	lcw.currentSuccessEmails = 0
	lcw.startTime = time.Now()
}

// UpdateUsageCounters updates internal counters (called during crawling)
func (lcw *LicensedCrawlerWrapper) UpdateUsageCounters(processed, success int) {
	lcw.currentProcessedEmails = processed
	lcw.currentSuccessEmails = success
}

// GetLicenseInfo returns enhanced license information
func (lcw *LicensedCrawlerWrapper) GetLicenseInfo() map[string]interface{} {
	info := lcw.licenseManager.GetLicenseInfo()

	// Add usage stats
	usageStats := lcw.GetUsageStats()
	for k, v := range usageStats {
		info[k] = v
	}

	return info
}

// ShowLicenseStatus shows detailed license status
func (lcw *LicensedCrawlerWrapper) ShowLicenseStatus() {
	info := lcw.GetLicenseInfo()

	fmt.Println("📋 LICENSE STATUS")
	fmt.Println("=================")

	if status, ok := info["status"].(string); ok {
		fmt.Printf("Status: %s\n", strings.ToUpper(status))
	}

	if userName, ok := info["user_name"].(string); ok {
		fmt.Printf("User: %s\n", userName)
	}

	if licenseType, ok := info["type"].(string); ok {
		fmt.Printf("Type: %s\n", strings.ToUpper(licenseType))
	}

	// Show usage
	if processed, ok := info["current_processed_emails"].(int); ok {
		if maxEmails, ok := info["max_emails"].(int); ok && maxEmails > 0 {
			fmt.Printf("Email Usage: %d/%d (%.1f%%)\n", processed, maxEmails,
				float64(processed)*100/float64(maxEmails))

			remaining := maxEmails - processed
			if remaining > 0 {
				fmt.Printf("Remaining: %d emails\n", remaining)
			} else {
				fmt.Println("⚠️ LIMIT REACHED")
			}
		} else {
			fmt.Printf("Email Usage: %d (Unlimited)\n", processed)
		}
	}

	if duration, ok := info["session_duration"].(string); ok {
		fmt.Printf("Session: %s\n", duration)
	}

	fmt.Println("=================")
}

// CheckFeatureAccess checks if specific feature is accessible
func (lcw *LicensedCrawlerWrapper) CheckFeatureAccess(feature string) bool {
	return lcw.licenseManager.CheckFeature(feature)
}

// ActivateLicense activates license with key
func (lcw *LicensedCrawlerWrapper) ActivateLicense(licenseKey string) error {
	err := lcw.licenseManager.SaveLicense(licenseKey)
	if err == nil {
		// Reset counters on new license activation
		lcw.ResetUsageCounters()
	}
	return err
}

// RemoveLicense removes current license
func (lcw *LicensedCrawlerWrapper) RemoveLicense() error {
	err := lcw.licenseManager.RemoveLicense()
	if err == nil {
		lcw.ResetUsageCounters()
	}
	return err
}

// handleLicenseError handles license validation errors
func (lcw *LicensedCrawlerWrapper) handleLicenseError(err error) error {
	fmt.Println("🔒 LICENSE VALIDATION FAILED")
	fmt.Println("===============================================")
	fmt.Printf("Error: %v\n", err)
	fmt.Println("")
	fmt.Println("📝 To activate your license:")
	fmt.Println("   1. Obtain a valid license key")
	fmt.Println("   2. Use the license activation feature in the GUI")
	fmt.Println("   3. Or contact support for assistance")
	fmt.Println("")
	fmt.Println("💡 License Types Available:")
	fmt.Println("   - TRIAL: 100 emails, 2 accounts, 30 days")
	fmt.Println("   - PERSONAL: 5,000 emails, 10 accounts, 1 year")
	fmt.Println("   - PRO: Unlimited emails & accounts, 1 year")
	fmt.Println("===============================================")

	return fmt.Errorf("license validation failed - please activate your license")
}

// showLicenseInfo displays enhanced license information
func (lcw *LicensedCrawlerWrapper) showLicenseInfo(info *LicenseInfo) {
	fmt.Println("✅ LICENSE VALIDATED")
	fmt.Println("===============================================")
	fmt.Printf("👤 User: %s (%s)\n", info.UserName, info.UserEmail)
	fmt.Printf("📄 Type: %s\n", strings.ToUpper(string(info.Type)))
	fmt.Printf("📅 Expires: %s\n", info.ExpiresAt.Format("2006-01-02"))

	daysLeft := int(time.Until(info.ExpiresAt).Hours() / 24)
	if daysLeft > 0 {
		fmt.Printf("⏰ Days left: %d\n", daysLeft)
	}

	// Show limits with current usage
	if info.MaxEmails > 0 {
		fmt.Printf("📧 Email limit: %d (Used: %d, Remaining: %d)\n",
			info.MaxEmails, lcw.currentProcessedEmails, info.MaxEmails-lcw.currentProcessedEmails)
	} else {
		fmt.Printf("📧 Email limit: Unlimited (Used: %d)\n", lcw.currentProcessedEmails)
	}

	if info.MaxAccounts > 0 {
		fmt.Printf("👥 Account limit: %d\n", info.MaxAccounts)
	} else {
		fmt.Printf("👥 Account limit: Unlimited\n")
	}

	fmt.Printf("🎯 Features: %s\n", strings.Join(info.Features, ", "))
	fmt.Println("===============================================")

	// Warning for expiring licenses
	if daysLeft <= 7 && daysLeft > 0 {
		fmt.Printf("⚠️  WARNING: Your license expires in %d days!\n", daysLeft)
		fmt.Println("   Please renew to continue using the software.")
		fmt.Println("")
	}

	// Warning for approaching email limits
	if info.MaxEmails > 0 && lcw.currentProcessedEmails > 0 {
		usagePercent := float64(lcw.currentProcessedEmails) * 100 / float64(info.MaxEmails)
		if usagePercent > 80 {
			fmt.Printf("⚠️  WARNING: %d%% of email quota used (%d/%d)\n",
				int(usagePercent), lcw.currentProcessedEmails, info.MaxEmails)
			fmt.Println("   Consider upgrading for more email processing capacity.")
			fmt.Println("")
		}
	}
}
