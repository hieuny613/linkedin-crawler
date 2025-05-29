package main

import (
	"crypto/md5"
	"fmt"
	"strings"
	"time"
)

type LicenseType string

const (
	LicenseTypeTrial    LicenseType = "trial"
	LicenseTypePersonal LicenseType = "personal"
	LicenseTypePro      LicenseType = "pro"
)

type LicenseManager struct {
	secretKey string
}

func NewLicenseManager() *LicenseManager {
	return &LicenseManager{
		secretKey: "LinkedIn-Crawler-2024-Security-Key",
	}
}

// generateLicenseChecksum - EXACT same as your current code
func (lm *LicenseManager) generateLicenseChecksum(licenseType LicenseType, userName, userEmail, expiryStr string) string {
	data := fmt.Sprintf("%s-%s-%s-%s-%s", licenseType, userName, userEmail, expiryStr, lm.secretKey)
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%X", hash)[:8] // Take first 8 characters
}

// GenerateLicenseKey - EXACT same as your current code
func GenerateLicenseKey(licenseType LicenseType, userName, userEmail string, validDays int) string {
	// Calculate expiry date
	expiryDate := time.Now().AddDate(0, 0, validDays)
	expiryStr := expiryDate.Format("20060102")

	// Generate checksum
	lm := NewLicenseManager()
	checksum := lm.generateLicenseChecksum(licenseType, userName, userEmail, expiryStr)

	// Format license key
	licenseKey := fmt.Sprintf("%s-%s-%s-%s-%s",
		strings.ToUpper(string(licenseType)),
		strings.ToUpper(userName),
		userEmail,
		expiryStr,
		checksum)

	return licenseKey
}

// Test function to validate key
func TestKey(licenseKey string) {
	lm := NewLicenseManager()

	fmt.Printf("🧪 Testing key: %s\n", licenseKey)

	parts := strings.Split(licenseKey, "-")
	if len(parts) < 5 {
		fmt.Printf("❌ Invalid format - need 5 parts, got %d\n", len(parts))
		return
	}

	licenseTypeStr := strings.ToLower(parts[0])
	userName := parts[1]
	userEmail := parts[2]
	expiryStr := parts[3]
	providedChecksum := strings.Join(parts[4:], "-")

	var licenseType LicenseType
	switch licenseTypeStr {
	case "trial":
		licenseType = LicenseTypeTrial
	case "personal":
		licenseType = LicenseTypePersonal
	case "pro":
		licenseType = LicenseTypePro
	default:
		fmt.Printf("❌ Invalid license type: %s\n", licenseTypeStr)
		return
	}

	// Generate expected checksum using EXACT same method
	expectedChecksum := lm.generateLicenseChecksum(licenseType, userName, userEmail, expiryStr)

	fmt.Printf("   Type: %s -> %s\n", parts[0], licenseType)
	fmt.Printf("   User: %s\n", userName)
	fmt.Printf("   Email: %s\n", userEmail)
	fmt.Printf("   Expiry: %s\n", expiryStr)
	fmt.Printf("   Expected checksum: %s\n", expectedChecksum)
	fmt.Printf("   Provided checksum: %s\n", providedChecksum)

	// Show checksum calculation data
	checksumData := fmt.Sprintf("%s-%s-%s-%s-%s", licenseType, userName, userEmail, expiryStr, lm.secretKey)
	fmt.Printf("   Checksum data: %s\n", checksumData)

	if expectedChecksum == providedChecksum {
		fmt.Printf("✅ VALID - Checksum matches!\n")
	} else {
		fmt.Printf("❌ INVALID - Checksum mismatch!\n")
	}
	fmt.Println()
}

func main() {
	fmt.Println("🔑 LINKEDIN CRAWLER LICENSE KEY TESTER")
	fmt.Println("=====================================")
	fmt.Println()

	// Generate working keys with today's date
	fmt.Println("🔧 Generating fresh license keys...")
	fmt.Println()

	// Trial license (30 days)
	trialKey := GenerateLicenseKey(LicenseTypeTrial, "TRIAL", "trial@example.com", 30)
	fmt.Printf("🆓 TRIAL LICENSE (30 days):\n%s\n\n", trialKey)

	// Personal license (365 days)
	personalKey := GenerateLicenseKey(LicenseTypePersonal, "JOHN", "john@company.com", 365)
	fmt.Printf("👤 PERSONAL LICENSE (1 year):\n%s\n\n", personalKey)

	// Pro license (365 days)
	proKey := GenerateLicenseKey(LicenseTypePro, "ADMIN", "admin@enterprise.com", 365)
	fmt.Printf("🏢 PRO LICENSE (1 year):\n%s\n\n", proKey)

	fmt.Println("=====================================")
	fmt.Println("🧪 VALIDATION TESTS:")
	fmt.Println()

	// Test each generated key
	TestKey(trialKey)
	TestKey(personalKey)
	TestKey(proKey)

	fmt.Println("=====================================")
	fmt.Println("📋 COPY ANY KEY ABOVE TO TEST IN APP")
	fmt.Println("=====================================")

	// Show specific working keys for immediate testing
	fmt.Println("🎯 QUICK TEST KEYS:")

	// Generate a simple trial key for today + 30 days
	simpleKey := GenerateLicenseKey(LicenseTypeTrial, "TEST", "test@test.com", 30)
	fmt.Printf("Simple Trial: %s\n", simpleKey)

	// Generate with longer expiry for testing
	longKey := GenerateLicenseKey(LicenseTypePro, "USER", "user@example.com", 365)
	fmt.Printf("Pro License: %s\n", longKey)
}
