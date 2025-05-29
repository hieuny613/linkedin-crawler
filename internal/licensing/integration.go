// internal/licensing/integration.go
package licensing

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// LicensedCrawlerWrapper wraps the main application with license checks
type LicensedCrawlerWrapper struct {
	licenseManager *LicenseManager
}

// NewLicensedCrawlerWrapper creates a new licensed wrapper
func NewLicensedCrawlerWrapper() *LicensedCrawlerWrapper {
	return &LicensedCrawlerWrapper{
		licenseManager: NewLicenseManager(),
	}
}

// ValidateAndStart validates license and starts the application
func (lcw *LicensedCrawlerWrapper) ValidateAndStart() error {
	// Try to load existing license
	info, err := lcw.licenseManager.LoadLicense()
	if err != nil {
		return lcw.handleLicenseError(err)
	}

	if !info.IsValid {
		return fmt.Errorf("license is not valid or has expired")
	}

	// Show license info
	lcw.showLicenseInfo(info)

	// Check if GUI is allowed
	if !lcw.licenseManager.CheckFeature(FeatureGUIInterface) {
		log.Printf("⚠️ GUI feature not available in your license")
		return fmt.Errorf("GUI interface not available in your license type")
	}

	return nil
}

// CheckCrawlingLimits checks if the user can crawl with given limits
func (lcw *LicensedCrawlerWrapper) CheckCrawlingLimits(emailCount, accountCount int) error {
	maxEmails, maxAccounts, err := lcw.licenseManager.GetUsageLimits()
	if err != nil {
		return fmt.Errorf("license validation failed: %w", err)
	}

	if maxEmails > 0 && emailCount > maxEmails {
		return fmt.Errorf("email limit exceeded: %d/%d (upgrade your license for more)", emailCount, maxEmails)
	}

	if maxAccounts > 0 && accountCount > maxAccounts {
		return fmt.Errorf("account limit exceeded: %d/%d (upgrade your license for more)", accountCount, maxAccounts)
	}

	return nil
}

// CheckFeatureAccess checks if a specific feature is accessible
func (lcw *LicensedCrawlerWrapper) CheckFeatureAccess(feature string) bool {
	return lcw.licenseManager.CheckFeature(feature)
}

// ActivateLicense activates license with key
func (lcw *LicensedCrawlerWrapper) ActivateLicense(licenseKey string) error {
	return lcw.licenseManager.SaveLicense(licenseKey)
}

// GetLicenseInfo returns license information
func (lcw *LicensedCrawlerWrapper) GetLicenseInfo() map[string]interface{} {
	return lcw.licenseManager.GetLicenseInfo()
}

// RemoveLicense removes current license
func (lcw *LicensedCrawlerWrapper) RemoveLicense() error {
	return lcw.licenseManager.RemoveLicense()
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

// showLicenseInfo displays license information
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

	// Show limits
	if info.MaxEmails > 0 {
		fmt.Printf("📧 Email limit: %d\n", info.MaxEmails)
	} else {
		fmt.Printf("📧 Email limit: Unlimited\n")
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
}
