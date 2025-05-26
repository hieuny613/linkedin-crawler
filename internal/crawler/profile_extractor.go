package crawler

import (
	"encoding/json"
	"fmt"

	"linkedin-crawler/internal/models"
)

// ProfileExtractor handles LinkedIn profile data extraction
type ProfileExtractor struct{}

// NewProfileExtractor creates a new ProfileExtractor instance
func NewProfileExtractor() *ProfileExtractor {
	return &ProfileExtractor{}
}

// NewProfileExtractorForCrawler creates a ProfileExtractor for a LinkedInCrawler
func NewProfileExtractorForCrawler(lc *models.LinkedInCrawler) *ProfileExtractor {
	return NewProfileExtractor()
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

// WriteProfileToFile writes profile data to output file
func (pe *ProfileExtractor) WriteProfileToFile(lc *models.LinkedInCrawler, email string, profile models.ProfileData) error {
	lc.OutputMutex.Lock()
	defer lc.OutputMutex.Unlock()

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

	return nil
}
