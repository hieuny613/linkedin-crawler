// tools/license-keygen/main.go - Tool to generate license keys
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"linkedin-crawler/internal/licensing"
)

func main() {
	fmt.Println("🔐 LinkedIn Crawler License Key Generator")
	fmt.Println("=========================================")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	// Interactive mode
	for {
		fmt.Println("Choose an option:")
		fmt.Println("1. Generate single license key")
		fmt.Println("2. Generate batch license keys")
		fmt.Println("3. Validate license key")
		fmt.Println("4. Show license types info")
		fmt.Println("5. Exit")
		fmt.Print("\nEnter your choice (1-5): ")

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			generateSingleKey(reader)
		case "2":
			generateBatchKeys(reader)
		case "3":
			validateKey(reader)
		case "4":
			showLicenseTypesInfo()
		case "5":
			fmt.Println("Goodbye!")
			return
		default:
			fmt.Println("❌ Invalid choice. Please try again.")
		}
		fmt.Println()
	}
}

// generateSingleKey generates a single license key
func generateSingleKey(reader *bufio.Reader) {
	fmt.Println("\n📝 Generate Single License Key")
	fmt.Println("------------------------------")

	// Get license type
	licenseType := getLicenseType(reader)
	if licenseType == "" {
		return
	}

	// Get user name
	fmt.Print("Enter user name (e.g., JOHN): ")
	userName, _ := reader.ReadString('\n')
	userName = strings.TrimSpace(strings.ToUpper(userName))
	if userName == "" {
		fmt.Println("❌ User name cannot be empty")
		return
	}

	// Get email
	fmt.Print("Enter email address: ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)
	if email == "" {
		fmt.Println("❌ Email cannot be empty")
		return
	}

	// Get validity days
	validDays := getValidityDays(reader, licenseType)
	if validDays <= 0 {
		return
	}

	// Generate license key
	licenseKey := licensing.GenerateLicenseKey(licensing.LicenseType(licenseType), userName, email, validDays)

	// Display result
	fmt.Println("\n✅ License Key Generated Successfully!")
	fmt.Println("====================================")
	fmt.Printf("User: %s (%s)\n", userName, email)
	fmt.Printf("Type: %s\n", strings.ToUpper(licenseType))
	fmt.Printf("Valid for: %d days\n", validDays)
	fmt.Printf("Expires: %s\n", time.Now().AddDate(0, 0, validDays).Format("2006-01-02"))
	fmt.Println()
	fmt.Printf("LICENSE KEY:\n%s\n", licenseKey)
	fmt.Println()

	// Ask to save to file
	fmt.Print("Save to file? (y/n): ")
	save, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(save)) == "y" {
		saveKeyToFile(licenseKey, userName, email, licenseType, validDays)
	}
}

// generateBatchKeys generates multiple license keys
func generateBatchKeys(reader *bufio.Reader) {
	fmt.Println("\n📚 Generate Batch License Keys")
	fmt.Println("------------------------------")

	// Get license type
	licenseType := getLicenseType(reader)
	if licenseType == "" {
		return
	}

	// Get validity days
	validDays := getValidityDays(reader, licenseType)
	if validDays <= 0 {
		return
	}

	// Get number of keys
	fmt.Print("How many keys to generate? ")
	countStr, _ := reader.ReadString('\n')
	count, err := strconv.Atoi(strings.TrimSpace(countStr))
	if err != nil || count <= 0 {
		fmt.Println("❌ Invalid number")
		return
	}

	// Get base name
	fmt.Print("Enter base name (e.g., USER): ")
	baseName, _ := reader.ReadString('\n')
	baseName = strings.TrimSpace(strings.ToUpper(baseName))
	if baseName == "" {
		fmt.Println("❌ Base name cannot be empty")
		return
	}

	// Get base email domain
	fmt.Print("Enter email domain (e.g., company.com): ")
	emailDomain, _ := reader.ReadString('\n')
	emailDomain = strings.TrimSpace(emailDomain)
	if emailDomain == "" {
		fmt.Println("❌ Email domain cannot be empty")
		return
	}

	// Generate keys
	fmt.Printf("\n🔄 Generating %d license keys...\n", count)
	fmt.Println("================================")

	var keys []string
	for i := 1; i <= count; i++ {
		userName := fmt.Sprintf("%s%d", baseName, i)
		email := fmt.Sprintf("%s%d@%s", strings.ToLower(baseName), i, emailDomain)

		licenseKey := licensing.GenerateLicenseKey(licensing.LicenseType(licenseType), userName, email, validDays)
		keys = append(keys, licenseKey)

		fmt.Printf("%d. %s (%s) -> %s\n", i, userName, email, licenseKey)
	}

	// Save batch to file
	fmt.Printf("\n💾 Saving batch to file...\n")
	saveBatchToFile(keys, licenseType, validDays, count)
}

// validateKey validates a license key
func validateKey(reader *bufio.Reader) {
	fmt.Println("\n🔍 Validate License Key")
	fmt.Println("----------------------")

	fmt.Print("Enter license key to validate: ")
	licenseKey, _ := reader.ReadString('\n')
	licenseKey = strings.TrimSpace(licenseKey)

	if licenseKey == "" {
		fmt.Println("❌ License key cannot be empty")
		return
	}

	// Validate using license manager
	lm := licensing.NewLicenseManager()
	info, err := lm.ValidateLicenseKey(licenseKey)

	if err != nil {
		fmt.Printf("❌ License validation failed: %v\n", err)
		return
	}

	// Display validation result
	fmt.Println("\n✅ License Key Valid!")
	fmt.Println("====================")
	fmt.Printf("User: %s (%s)\n", info.UserName, info.UserEmail)
	fmt.Printf("Type: %s\n", strings.ToUpper(string(info.Type)))
	fmt.Printf("Expires: %s\n", info.ExpiresAt.Format("2006-01-02 15:04:05"))

	daysLeft := int(time.Until(info.ExpiresAt).Hours() / 24)
	if daysLeft > 0 {
		fmt.Printf("Days remaining: %d\n", daysLeft)
	} else {
		fmt.Printf("Status: EXPIRED (%d days ago)\n", -daysLeft)
	}

	// Show limits
	if info.MaxEmails > 0 {
		fmt.Printf("Email limit: %d\n", info.MaxEmails)
	} else {
		fmt.Printf("Email limit: Unlimited\n")
	}

	if info.MaxAccounts > 0 {
		fmt.Printf("Account limit: %d\n", info.MaxAccounts)
	} else {
		fmt.Printf("Account limit: Unlimited\n")
	}

	fmt.Printf("Features: %s\n", strings.Join(info.Features, ", "))
}

// showLicenseTypesInfo shows information about license types
func showLicenseTypesInfo() {
	fmt.Println("\n📋 License Types Information")
	fmt.Println("============================")

	fmt.Println("\n🆓 TRIAL License:")
	fmt.Println("   • Duration: Typically 30 days")
	fmt.Println("   • Email limit: 100 emails")
	fmt.Println("   • Account limit: 2 accounts")
	fmt.Println("   • Features: Basic crawling, GUI interface")
	fmt.Println("   • Best for: Testing the software")

	fmt.Println("\n👤 PERSONAL License:")
	fmt.Println("   • Duration: Typically 365 days (1 year)")
	fmt.Println("   • Email limit: 5,000 emails")
	fmt.Println("   • Account limit: 10 accounts")
	fmt.Println("   • Features: All trial features + bulk processing, export tools")
	fmt.Println("   • Best for: Individual users, small projects")

	fmt.Println("\n🏢 PRO License:")
	fmt.Println("   • Duration: Typically 365 days (1 year)")
	fmt.Println("   • Email limit: Unlimited")
	fmt.Println("   • Account limit: Unlimited")
	fmt.Println("   • Features: All features + advanced crawling, priority support")
	fmt.Println("   • Best for: Businesses, large-scale operations")

	fmt.Println("\n🔑 License Key Format:")
	fmt.Println("   TYPE-USERNAME-EMAIL-EXPIRY-CHECKSUM")
	fmt.Println("   Example: PRO-COMPANY-admin@company.com-20251201-ABC123")
}

// getLicenseType prompts for license type selection
func getLicenseType(reader *bufio.Reader) string {
	for {
		fmt.Println("\nSelect license type:")
		fmt.Println("1. TRIAL (100 emails, 2 accounts)")
		fmt.Println("2. PERSONAL (5,000 emails, 10 accounts)")
		fmt.Println("3. PRO (unlimited)")
		fmt.Print("Enter choice (1-3): ")

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			return "trial"
		case "2":
			return "personal"
		case "3":
			return "pro"
		default:
			fmt.Println("❌ Invalid choice. Please try again.")
		}
	}
}

// getValidityDays prompts for validity period
func getValidityDays(reader *bufio.Reader, licenseType string) int {
	// Suggest default based on license type
	var defaultDays int
	switch licenseType {
	case "trial":
		defaultDays = 30
	case "personal", "pro":
		defaultDays = 365
	}

	fmt.Printf("Enter validity period in days (default %d): ", defaultDays)
	daysStr, _ := reader.ReadString('\n')
	daysStr = strings.TrimSpace(daysStr)

	if daysStr == "" {
		return defaultDays
	}

	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		fmt.Println("❌ Invalid number of days")
		return 0
	}

	return days
}

// saveKeyToFile saves a single license key to file
func saveKeyToFile(licenseKey, userName, email, licenseType string, validDays int) {
	filename := fmt.Sprintf("license_%s_%s_%s.txt",
		licenseType,
		strings.ToLower(userName),
		time.Now().Format("20060102"))

	content := fmt.Sprintf(`LinkedIn Crawler License Key
============================

User: %s
Email: %s
Type: %s
Valid for: %d days
Generated: %s
Expires: %s

LICENSE KEY:
%s

Instructions:
1. Copy the license key above
2. Open LinkedIn Crawler
3. Go to License tab
4. Paste the key and click "Activate License"

Support: contact your license provider for assistance
`, userName, email, strings.ToUpper(licenseType), validDays,
		time.Now().Format("2006-01-02 15:04:05"),
		time.Now().AddDate(0, 0, validDays).Format("2006-01-02"),
		licenseKey)

	err := os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		fmt.Printf("❌ Failed to save file: %v\n", err)
	} else {
		fmt.Printf("✅ License saved to: %s\n", filename)
	}
}

// saveBatchToFile saves batch license keys to file
func saveBatchToFile(keys []string, licenseType string, validDays, count int) {
	filename := fmt.Sprintf("license_batch_%s_%d_keys_%s.txt",
		licenseType,
		count,
		time.Now().Format("20060102"))

	content := fmt.Sprintf(`LinkedIn Crawler License Keys - Batch Generation
===============================================

License Type: %s
Generated: %s
Valid for: %d days
Expires: %s
Total keys: %d

LICENSE KEYS:
`, strings.ToUpper(licenseType),
		time.Now().Format("2006-01-02 15:04:05"),
		validDays,
		time.Now().AddDate(0, 0, validDays).Format("2006-01-02"),
		count)

	for i, key := range keys {
		content += fmt.Sprintf("%d. %s\n", i+1, key)
	}

	content += fmt.Sprintf(`
Instructions:
1. Distribute these keys to your users
2. Each user should copy their assigned key
3. Open LinkedIn Crawler
4. Go to License tab  
5. Paste the key and click "Activate License"

Support: contact your license provider for assistance
`)

	err := os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		fmt.Printf("❌ Failed to save batch file: %v\n", err)
	} else {
		fmt.Printf("✅ Batch licenses saved to: %s\n", filename)
	}

	// Also save as CSV for easy distribution
	csvFilename := strings.Replace(filename, ".txt", ".csv", 1)
	csvContent := "User,Email,License_Key,Type,Valid_Days,Expires\n"

	for _, key := range keys {
		// Parse key to extract info
		parts := strings.Split(key, "-")
		if len(parts) >= 3 {
			userName := parts[1]
			email := parts[2]
			expiryDate := time.Now().AddDate(0, 0, validDays).Format("2006-01-02")

			csvContent += fmt.Sprintf("%s,%s,%s,%s,%d,%s\n",
				userName, email, key, strings.ToUpper(licenseType), validDays, expiryDate)
		}
	}

	err = os.WriteFile(csvFilename, []byte(csvContent), 0644)
	if err != nil {
		fmt.Printf("❌ Failed to save CSV file: %v\n", err)
	} else {
		fmt.Printf("✅ CSV format saved to: %s\n", csvFilename)
	}
}
