// internal/licensing/license.go
package licensing

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LicenseType represents different license types
type LicenseType string

const (
	LicenseTypeTrial    LicenseType = "trial"
	LicenseTypePersonal LicenseType = "personal"
	LicenseTypePro      LicenseType = "pro"
)

// LicenseInfo represents license information
type LicenseInfo struct {
	Type        LicenseType `json:"type"`
	UserName    string      `json:"user_name"`
	UserEmail   string      `json:"user_email"`
	ExpiresAt   time.Time   `json:"expires_at"`
	MaxEmails   int         `json:"max_emails"`
	MaxAccounts int         `json:"max_accounts"`
	Features    []string    `json:"features"`
	IsValid     bool        `json:"is_valid"`
}

// LicenseManager handles offline license validation
type LicenseManager struct {
	licenseFile string
	secretKey   string
}

// NewLicenseManager creates a new license manager
func NewLicenseManager() *LicenseManager {
	return &LicenseManager{
		licenseFile: "license.key",
		secretKey:   "LinkedIn-Crawler-2024-Security-Key", // Your secret key
	}
}

// ValidateLicenseKey validates a license key and returns license info
func (lm *LicenseManager) ValidateLicenseKey(licenseKey string) (*LicenseInfo, error) {
	// Clean license key - ONLY remove spaces, keep dashes
	licenseKey = strings.ReplaceAll(licenseKey, " ", "")
	licenseKey = strings.TrimSpace(licenseKey)

	// Decode license key
	info, err := lm.decodeLicenseKey(licenseKey)
	if err != nil {
		return nil, fmt.Errorf("invalid license key format: %w", err)
	}

	// Check expiration
	if time.Now().After(info.ExpiresAt) {
		info.IsValid = false
		return info, fmt.Errorf("license has expired on %s", info.ExpiresAt.Format("2006-01-02"))
	}

	info.IsValid = true
	return info, nil
}

// SaveLicense saves license to file
func (lm *LicenseManager) SaveLicense(licenseKey string) error {
	// Validate first
	info, err := lm.ValidateLicenseKey(licenseKey)
	if err != nil {
		return err
	}

	// Create license data
	licenseData := map[string]interface{}{
		"key":      licenseKey,
		"saved_at": time.Now(),
		"info":     info,
		"checksum": lm.generateChecksum(licenseKey),
	}

	// Save to file
	return lm.saveLicenseFile(licenseData)
}

// LoadLicense loads and validates saved license
func (lm *LicenseManager) LoadLicense() (*LicenseInfo, error) {
	// Check if license file exists
	if _, err := os.Stat(lm.licenseFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("no license found - please enter your license key")
	}

	// Load license data
	licenseData, err := lm.loadLicenseFile()
	if err != nil {
		return nil, fmt.Errorf("failed to load license: %w", err)
	}

	// Extract license key
	licenseKey, ok := licenseData["key"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid license file format")
	}

	// Verify checksum
	savedChecksum, ok := licenseData["checksum"].(string)
	if !ok || savedChecksum != lm.generateChecksum(licenseKey) {
		return nil, fmt.Errorf("license file has been tampered with")
	}

	// Validate license key
	return lm.ValidateLicenseKey(licenseKey)
}

// CheckFeature checks if a feature is available
func (lm *LicenseManager) CheckFeature(feature string) bool {
	info, err := lm.LoadLicense()
	if err != nil {
		return false
	}

	for _, f := range info.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// GetUsageLimits returns usage limits
func (lm *LicenseManager) GetUsageLimits() (maxEmails, maxAccounts int, err error) {
	info, err := lm.LoadLicense()
	if err != nil {
		return 0, 0, err
	}

	return info.MaxEmails, info.MaxAccounts, nil
}

// GetLicenseInfo returns current license information
func (lm *LicenseManager) GetLicenseInfo() map[string]interface{} {
	info, err := lm.LoadLicense()
	if err != nil {
		return map[string]interface{}{
			"status": "invalid",
			"error":  err.Error(),
		}
	}

	daysLeft := int(time.Until(info.ExpiresAt).Hours() / 24)
	status := "active"
	if daysLeft <= 0 {
		status = "expired"
	} else if daysLeft <= 7 {
		status = "expiring_soon"
	}

	return map[string]interface{}{
		"status":       status,
		"type":         string(info.Type),
		"user_name":    info.UserName,
		"user_email":   info.UserEmail,
		"expires_at":   info.ExpiresAt.Format("2006-01-02 15:04:05"),
		"days_left":    daysLeft,
		"max_emails":   info.MaxEmails,
		"max_accounts": info.MaxAccounts,
		"features":     info.Features,
	}
}

// RemoveLicense removes the license file
func (lm *LicenseManager) RemoveLicense() error {
	return os.Remove(lm.licenseFile)
}

// decodeLicenseKey decodes and validates license key format
func (lm *LicenseManager) decodeLicenseKey(licenseKey string) (*LicenseInfo, error) {
	// License key format: BASE64(JSON + SIGNATURE)
	// Example: TRIAL-JOHN-DOE-20241201-SIGNATURE

	if len(licenseKey) < 20 {
		return nil, fmt.Errorf("license key too short")
	}

	// Try to decode as base64
	decoded, err := base64.StdEncoding.DecodeString(licenseKey)
	if err != nil {
		// If not base64, try to parse as custom format
		return lm.parseCustomLicenseKey(licenseKey)
	}

	// Parse JSON from decoded data
	var info LicenseInfo
	if err := json.Unmarshal(decoded, &info); err != nil {
		return nil, fmt.Errorf("invalid license data")
	}

	return &info, nil
}

// parseCustomLicenseKey parses custom license key format
func (lm *LicenseManager) parseCustomLicenseKey(licenseKey string) (*LicenseInfo, error) {
	// Custom format: TYPE-USERNAME-EMAIL-EXPIRY-CHECKSUM
	// Example: PRO-JOHN-john@email.com-20241201-ABC123

	parts := strings.Split(licenseKey, "-")
	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid license key format - expected 5 parts, got %d", len(parts))
	}

	// Parse license type
	licenseTypeStr := strings.ToLower(parts[0])
	var licenseType LicenseType
	switch licenseTypeStr {
	case "trial":
		licenseType = LicenseTypeTrial
	case "personal":
		licenseType = LicenseTypePersonal
	case "pro":
		licenseType = LicenseTypePro
	default:
		return nil, fmt.Errorf("invalid license type: %s (must be TRIAL, PERSONAL, or PRO)", parts[0])
	}

	// Parse user info
	userName := strings.ToUpper(parts[1])
	userEmail := strings.ToLower(parts[2])

	// Validate email format
	if !strings.Contains(userEmail, "@") || !strings.Contains(userEmail, ".") {
		return nil, fmt.Errorf("invalid email format: %s", userEmail)
	}

	// Parse expiry date
	expiryStr := parts[3]
	if len(expiryStr) != 8 { // YYYYMMDD format
		return nil, fmt.Errorf("invalid expiry date format: %s (expected YYYYMMDD)", expiryStr)
	}

	expiryDate, err := time.Parse("20060102", expiryStr)
	if err != nil {
		return nil, fmt.Errorf("invalid expiry date: %s (%v)", expiryStr, err)
	}

	// Verify checksum - join remaining parts in case checksum contains dashes
	providedChecksum := strings.Join(parts[4:], "-")
	expectedChecksum := lm.generateLicenseChecksum(licenseType, userName, userEmail, expiryStr)

	if expectedChecksum != providedChecksum {
		return nil, fmt.Errorf("invalid license checksum - license key may be corrupted or tampered with")
	}

	// Set limits and features based on license type
	info := &LicenseInfo{
		Type:      licenseType,
		UserName:  userName,
		UserEmail: userEmail,
		ExpiresAt: expiryDate,
	}

	// Set limits based on license type
	switch licenseType {
	case LicenseTypeTrial:
		info.MaxEmails = 100
		info.MaxAccounts = 2
		info.Features = []string{"basic_crawling", "gui_interface"}
	case LicenseTypePersonal:
		info.MaxEmails = 5000
		info.MaxAccounts = 10
		info.Features = []string{"basic_crawling", "gui_interface", "export_tools", "bulk_processing"}
	case LicenseTypePro:
		info.MaxEmails = -1   // Unlimited
		info.MaxAccounts = -1 // Unlimited
		info.Features = []string{"basic_crawling", "gui_interface", "export_tools", "bulk_processing", "advanced_crawling", "priority_support"}
	}

	return info, nil
}

// generateLicenseChecksum generates checksum for license validation
func (lm *LicenseManager) generateLicenseChecksum(licenseType LicenseType, userName, userEmail, expiryStr string) string {
	// Create consistent data string for checksum
	data := fmt.Sprintf("%s|%s|%s|%s|%s",
		strings.ToUpper(string(licenseType)),
		strings.ToUpper(userName),
		strings.ToLower(userEmail),
		expiryStr,
		lm.secretKey)
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%X", hash)[:8] // Take first 8 characters
}

// generateChecksum generates checksum for file integrity
func (lm *LicenseManager) generateChecksum(data string) string {
	combined := data + lm.secretKey
	hash := sha256.Sum256([]byte(combined))
	return fmt.Sprintf("%x", hash)[:16] // Take first 16 characters
}

// saveLicenseFile saves license data to encrypted file
func (lm *LicenseManager) saveLicenseFile(data map[string]interface{}) error {
	// Create directory if not exists
	dir := filepath.Dir(lm.licenseFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	// Simple encryption (XOR with key)
	encrypted := lm.xorEncrypt(jsonData, lm.secretKey)

	// Encode as base64
	encoded := base64.StdEncoding.EncodeToString(encrypted)

	// Write to file
	return os.WriteFile(lm.licenseFile, []byte(encoded), 0600)
}

// loadLicenseFile loads and decrypts license file
func (lm *LicenseManager) loadLicenseFile() (map[string]interface{}, error) {
	// Read file
	data, err := os.ReadFile(lm.licenseFile)
	if err != nil {
		return nil, err
	}

	// Decode from base64
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid license file format")
	}

	// Decrypt
	decrypted := lm.xorEncrypt(decoded, lm.secretKey)

	// Parse JSON
	var result map[string]interface{}
	if err := json.Unmarshal(decrypted, &result); err != nil {
		return nil, fmt.Errorf("corrupted license file")
	}

	return result, nil
}

// xorEncrypt performs XOR encryption/decryption
func (lm *LicenseManager) xorEncrypt(data []byte, key string) []byte {
	keyBytes := []byte(key)
	result := make([]byte, len(data))

	for i, b := range data {
		result[i] = b ^ keyBytes[i%len(keyBytes)]
	}

	return result
}

// Feature constants
const (
	FeatureBasicCrawling    = "basic_crawling"
	FeatureAdvancedCrawling = "advanced_crawling"
	FeatureBulkProcessing   = "bulk_processing"
	FeatureGUIInterface     = "gui_interface"
	FeatureExportTools      = "export_tools"
	FeaturePrioritySupport  = "priority_support"
)

// GenerateLicenseKey generates a license key (for your internal use)
func GenerateLicenseKey(licenseType LicenseType, userName, userEmail string, validDays int) string {
	// Calculate expiry date
	expiryDate := time.Now().AddDate(0, 0, validDays)
	expiryStr := expiryDate.Format("20060102")

	// Normalize inputs for consistent checksum
	normalizedType := strings.ToUpper(string(licenseType))
	normalizedUser := strings.ToUpper(userName)
	normalizedEmail := strings.ToLower(userEmail)

	// Generate checksum using same method as validation
	lm := NewLicenseManager()
	checksum := lm.generateLicenseChecksum(licenseType, normalizedUser, normalizedEmail, expiryStr)

	// Format license key
	licenseKey := fmt.Sprintf("%s-%s-%s-%s-%s",
		normalizedType,
		normalizedUser,
		normalizedEmail,
		expiryStr,
		checksum)

	return licenseKey
}

// Example usage and testing functions
func ExampleGenerateLicenseKeys() {
	fmt.Println("Example License Keys:")
	fmt.Println("Trial (30 days):", GenerateLicenseKey(LicenseTypeTrial, "JOHN", "john@example.com", 30))
	fmt.Println("Personal (365 days):", GenerateLicenseKey(LicenseTypePersonal, "JANE", "jane@example.com", 365))
	fmt.Println("Pro (365 days):", GenerateLicenseKey(LicenseTypePro, "COMPANY", "admin@company.com", 365))
}
