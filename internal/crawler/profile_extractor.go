package crawler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"linkedin-crawler/internal/models"
)

// ProfileExtractor handles LinkedIn profile data extraction
type ProfileExtractor struct {
	// Cache để tránh ghi trùng vào hit.txt
	writtenProfiles map[string]bool
	profilesMutex   sync.RWMutex
}

// NewProfileExtractor creates a new ProfileExtractor instance
func NewProfileExtractor() *ProfileExtractor {
	pe := &ProfileExtractor{
		writtenProfiles: make(map[string]bool),
	}

	// Load existing profiles from hit.txt để tránh ghi trùng
	pe.loadExistingProfiles()

	return pe
}

// NewProfileExtractorForCrawler creates a ProfileExtractor for a LinkedInCrawler
func NewProfileExtractorForCrawler(lc *models.LinkedInCrawler) *ProfileExtractor {
	return NewProfileExtractor()
}

// loadExistingProfiles loads existing emails from hit.txt to avoid duplicates
func (pe *ProfileExtractor) loadExistingProfiles() {
	file, err := os.Open("hit.txt")
	if err != nil {
		// File doesn't exist, that's fine
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	loadedCount := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse existing entries: email|name|url|location|connections
		parts := strings.Split(line, "|")
		if len(parts) >= 1 {
			email := strings.TrimSpace(parts[0])
			if email != "" {
				pe.writtenProfiles[strings.ToLower(email)] = true
				loadedCount++
			}
		}
	}

	if loadedCount > 0 {
		fmt.Printf("🔄 ProfileExtractor: Loaded %d existing profiles to prevent duplicates\n", loadedCount)
	}
}

// ExtractProfileData extracts LinkedIn profile data from response JSON
func (pe *ProfileExtractor) ExtractProfileData(responseJSON []byte) (models.ProfileData, error) {
	var data map[string]interface{}
	var profile models.ProfileData

	if err := json.Unmarshal(responseJSON, &data); err != nil {
		return profile, err
	}

	persons, ok := data["persons"].([]interface{})
	if !ok || len(persons) == 0 {
		return profile, nil
	}

	p, ok := persons[0].(map[string]interface{})
	if !ok {
		return profile, nil
	}

	if val, ok := p["displayName"].(string); ok {
		profile.User = val
	}

	if val, ok := p["linkedInUrl"].(string); ok {
		profile.LinkedInURL = val
	}

	if val, ok := p["connectionCount"].(string); ok {
		profile.ConnectionCount = val
	} else if val, ok := p["connectionCount"].(float64); ok {
		profile.ConnectionCount = fmt.Sprintf("%d", int(val))
	}

	if val, ok := p["location"].(string); ok {
		profile.Location = val
	}

	return profile, nil
}

// WriteProfileToFile writes profile data to output file with duplicate prevention
func (pe *ProfileExtractor) WriteProfileToFile(lc *models.LinkedInCrawler, email string, profile models.ProfileData) error {
	// Check if email already written to prevent duplicates
	emailKey := strings.ToLower(strings.TrimSpace(email))

	pe.profilesMutex.RLock()
	alreadyWritten := pe.writtenProfiles[emailKey]
	pe.profilesMutex.RUnlock()

	if alreadyWritten {
		fmt.Printf("⚠️ Skip duplicate: %s already exists in hit.txt\n", email)
		return nil // Skip duplicate
	}

	// Thread-safe file writing
	lc.OutputMutex.Lock()
	defer lc.OutputMutex.Unlock()

	// Double-check after acquiring lock (defensive programming)
	pe.profilesMutex.RLock()
	alreadyWritten = pe.writtenProfiles[emailKey]
	pe.profilesMutex.RUnlock()

	if alreadyWritten {
		fmt.Printf("⚠️ Skip duplicate (double-check): %s already exists in hit.txt\n", email)
		return nil
	}

	// APPEND mode - ghi thêm vào file hit.txt (KHÔNG ghi đè)
	line := fmt.Sprintf("%s|%s|%s|%s|%s\n", email, profile.User, profile.LinkedInURL, profile.Location, profile.ConnectionCount)
	_, err := lc.BufferedWriter.WriteString(line)
	if err != nil {
		return fmt.Errorf("failed to write to output file: %w", err)
	}

	// Force flush để đảm bảo data được ghi ngay lập tức
	if flushErr := lc.BufferedWriter.Flush(); flushErr != nil {
		return fmt.Errorf("failed to flush output file: %w", flushErr)
	}

	// Force sync to disk để tránh mất data khi crash
	if syncErr := lc.OutputFile.Sync(); syncErr != nil {
		return fmt.Errorf("failed to sync output file: %w", syncErr)
	}

	// Mark as written to prevent future duplicates
	pe.profilesMutex.Lock()
	pe.writtenProfiles[emailKey] = true
	pe.profilesMutex.Unlock()

	fmt.Printf("✅ Written to hit.txt: %s -> %s\n", email, profile.User)
	return nil
}

// IsProfileAlreadyWritten checks if a profile for this email has already been written
func (pe *ProfileExtractor) IsProfileAlreadyWritten(email string) bool {
	emailKey := strings.ToLower(strings.TrimSpace(email))

	pe.profilesMutex.RLock()
	defer pe.profilesMutex.RUnlock()

	return pe.writtenProfiles[emailKey]
}

// GetWrittenProfilesCount returns the number of profiles written to file
func (pe *ProfileExtractor) GetWrittenProfilesCount() int {
	pe.profilesMutex.RLock()
	defer pe.profilesMutex.RUnlock()

	return len(pe.writtenProfiles)
}

// ClearCache clears the internal cache (for testing purposes)
func (pe *ProfileExtractor) ClearCache() {
	pe.profilesMutex.Lock()
	defer pe.profilesMutex.Unlock()

	pe.writtenProfiles = make(map[string]bool)
}

// RefreshCache reloads existing profiles from file (useful after external file changes)
func (pe *ProfileExtractor) RefreshCache() {
	pe.profilesMutex.Lock()
	defer pe.profilesMutex.Unlock()

	// Clear current cache
	pe.writtenProfiles = make(map[string]bool)

	// Reload from file
	pe.profilesMutex.Unlock() // Temporarily unlock for loadExistingProfiles
	pe.loadExistingProfiles()
	pe.profilesMutex.Lock() // Re-lock before defer unlock
}

// WriteProfileToFileWithValidation writes profile with additional validation
func (pe *ProfileExtractor) WriteProfileToFileWithValidation(lc *models.LinkedInCrawler, email string, profile models.ProfileData) error {
	// Validate email format
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}

	// Validate profile data
	if profile.User == "" || profile.User == "null" || profile.User == "{}" {
		return fmt.Errorf("invalid profile data: user is empty or null")
	}

	// Write to file
	return pe.WriteProfileToFile(lc, email, profile)
}

// GetCacheStats returns statistics about the cache
func (pe *ProfileExtractor) GetCacheStats() map[string]interface{} {
	pe.profilesMutex.RLock()
	defer pe.profilesMutex.RUnlock()

	stats := map[string]interface{}{
		"total_profiles_cached": len(pe.writtenProfiles),
		"cache_enabled":         true,
	}

	return stats
}
