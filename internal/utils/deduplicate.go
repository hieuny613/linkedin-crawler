package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// HitResult represents a result entry in hit.txt
type HitResult struct {
	Email       string
	Name        string
	LinkedInURL string
	Location    string
	Connections string
	Timestamp   time.Time // For tracking when added
}

// DeduplicateHitFile removes duplicate entries from hit.txt file
func DeduplicateHitFile(filePath string) error {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file %s does not exist", filePath)
	}

	// Read existing entries
	entries, err := readHitFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read hit file: %w", err)
	}

	originalCount := len(entries)
	if originalCount == 0 {
		return nil // Nothing to deduplicate
	}

	// Remove duplicates using map
	uniqueEntries := make(map[string]HitResult) // key = email (lowercase)
	duplicatesCount := 0

	for _, entry := range entries {
		emailKey := strings.ToLower(strings.TrimSpace(entry.Email))

		if existing, exists := uniqueEntries[emailKey]; exists {
			duplicatesCount++
			// Keep the entry with more LinkedIn info or newer timestamp
			if (entry.LinkedInURL != "" && entry.LinkedInURL != "N/A") &&
				(existing.LinkedInURL == "" || existing.LinkedInURL == "N/A") {
				uniqueEntries[emailKey] = entry
			} else if entry.Timestamp.After(existing.Timestamp) {
				uniqueEntries[emailKey] = entry
			}
			// Otherwise keep the existing one
		} else {
			uniqueEntries[emailKey] = entry
		}
	}

	// Convert back to slice
	deduplicatedEntries := make([]HitResult, 0, len(uniqueEntries))
	for _, entry := range uniqueEntries {
		deduplicatedEntries = append(deduplicatedEntries, entry)
	}

	// Write back to file
	err = writeHitFile(filePath, deduplicatedEntries)
	if err != nil {
		return fmt.Errorf("failed to write deduplicated file: %w", err)
	}

	fmt.Printf("✅ Deduplicated %s: %d → %d entries (removed %d duplicates)\n",
		filePath, originalCount, len(deduplicatedEntries), duplicatesCount)

	return nil
}

// readHitFile reads entries from hit.txt file
func readHitFile(filePath string) ([]HitResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []HitResult
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse line: email|name|linkedin_url|location|connections
		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			fmt.Printf("⚠️ Line %d: Invalid format, skipping: %s\n", lineNum, line)
			continue
		}

		entry := HitResult{
			Email:       strings.TrimSpace(parts[0]),
			Name:        strings.TrimSpace(parts[1]),
			LinkedInURL: strings.TrimSpace(parts[2]),
			Location:    strings.TrimSpace(parts[3]),
			Connections: strings.TrimSpace(parts[4]),
			Timestamp:   time.Now(), // Use current time as default
		}

		// Basic validation
		if entry.Email == "" {
			fmt.Printf("⚠️ Line %d: Empty email, skipping: %s\n", lineNum, line)
			continue
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return entries, nil
}

// writeHitFile writes entries to hit.txt file
func writeHitFile(filePath string, entries []HitResult) error {
	// Create backup of original file
	backupPath := filePath + ".backup." + time.Now().Format("20060102-150405")
	if _, err := os.Stat(filePath); err == nil {
		if err := copyFile(filePath, backupPath); err != nil {
			fmt.Printf("⚠️ Could not create backup: %v\n", err)
		} else {
			fmt.Printf("💾 Created backup: %s\n", backupPath)
		}
	}

	// Write deduplicated entries
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	// Write header
	writer.WriteString("# LinkedIn Profile Results\n")
	writer.WriteString(fmt.Sprintf("# Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	writer.WriteString(fmt.Sprintf("# Total entries: %d\n", len(entries)))
	writer.WriteString("# Format: email|name|linkedin_url|location|connections\n")
	writer.WriteString("\n")

	// Write entries
	for _, entry := range entries {
		line := fmt.Sprintf("%s|%s|%s|%s|%s\n",
			entry.Email, entry.Name, entry.LinkedInURL, entry.Location, entry.Connections)
		writer.WriteString(line)
	}

	return nil
}

// copyFile creates a copy of a file
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	scanner := bufio.NewScanner(sourceFile)
	writer := bufio.NewWriter(destFile)
	defer writer.Flush()

	for scanner.Scan() {
		writer.WriteString(scanner.Text() + "\n")
	}

	return scanner.Err()
}

// GetHitFileStats returns statistics about hit.txt file
func GetHitFileStats(filePath string) (map[string]int, error) {
	stats := make(map[string]int)

	entries, err := readHitFile(filePath)
	if err != nil {
		return stats, err
	}

	// Count unique emails and detect duplicates
	emailMap := make(map[string]int)
	withLinkedIn := 0

	for _, entry := range entries {
		emailKey := strings.ToLower(strings.TrimSpace(entry.Email))
		emailMap[emailKey]++

		if entry.LinkedInURL != "" && entry.LinkedInURL != "N/A" {
			withLinkedIn++
		}
	}

	duplicates := 0
	for _, count := range emailMap {
		if count > 1 {
			duplicates += count - 1
		}
	}

	stats["total_entries"] = len(entries)
	stats["unique_emails"] = len(emailMap)
	stats["duplicates"] = duplicates
	stats["with_linkedin"] = withLinkedIn
	stats["without_linkedin"] = len(entries) - withLinkedIn

	return stats, nil
}

// AutoDeduplicateOnStartup automatically deduplicates hit.txt on application startup
func AutoDeduplicateOnStartup() {
	filePath := "hit.txt"

	// Check if file exists and has content
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return // File doesn't exist, nothing to deduplicate
	}

	// Get stats before deduplication
	statsBefore, err := GetHitFileStats(filePath)
	if err != nil {
		fmt.Printf("⚠️ Could not analyze hit.txt: %v\n", err)
		return
	}

	// Only deduplicate if there are duplicates
	if statsBefore["duplicates"] > 0 {
		fmt.Printf("🔄 Auto-deduplicating hit.txt: %d duplicates detected\n", statsBefore["duplicates"])

		err := DeduplicateHitFile(filePath)
		if err != nil {
			fmt.Printf("⚠️ Auto-deduplication failed: %v\n", err)
		} else {
			// Get stats after deduplication
			statsAfter, err := GetHitFileStats(filePath)
			if err == nil {
				fmt.Printf("✅ Auto-deduplication complete: %d → %d entries\n",
					statsBefore["total_entries"], statsAfter["total_entries"])
			}
		}
	}
}

// ValidateHitFile validates the format and content of hit.txt file
func ValidateHitFile(filePath string) []string {
	var issues []string

	entries, err := readHitFile(filePath)
	if err != nil {
		issues = append(issues, fmt.Sprintf("Could not read file: %v", err))
		return issues
	}

	if len(entries) == 0 {
		issues = append(issues, "File is empty")
		return issues
	}

	// Check for duplicates
	emailMap := make(map[string][]int) // email -> line numbers
	invalidEmails := 0
	emptyNames := 0
	emptyUrls := 0

	for i, entry := range entries {
		lineNum := i + 1
		emailKey := strings.ToLower(strings.TrimSpace(entry.Email))

		if emailKey == "" {
			invalidEmails++
			continue
		}

		emailMap[emailKey] = append(emailMap[emailKey], lineNum)

		if entry.Name == "" || entry.Name == "null" {
			emptyNames++
		}

		if entry.LinkedInURL == "" || entry.LinkedInURL == "N/A" {
			emptyUrls++
		}
	}

	// Report duplicates
	duplicateGroups := 0
	totalDuplicates := 0
	for email, lineNumbers := range emailMap {
		if len(lineNumbers) > 1 {
			duplicateGroups++
			totalDuplicates += len(lineNumbers) - 1
			if duplicateGroups <= 5 { // Show first 5 duplicate groups
				issues = append(issues, fmt.Sprintf("Duplicate email '%s' found on lines: %v", email, lineNumbers))
			}
		}
	}

	if duplicateGroups > 5 {
		issues = append(issues, fmt.Sprintf("... and %d more duplicate groups", duplicateGroups-5))
	}

	// Summary
	if totalDuplicates > 0 {
		issues = append(issues, fmt.Sprintf("Total: %d duplicate entries found", totalDuplicates))
	}

	if invalidEmails > 0 {
		issues = append(issues, fmt.Sprintf("Invalid/empty emails: %d", invalidEmails))
	}

	if emptyNames > 0 {
		issues = append(issues, fmt.Sprintf("Empty names: %d", emptyNames))
	}

	if emptyUrls > 0 {
		issues = append(issues, fmt.Sprintf("Empty/N/A LinkedIn URLs: %d", emptyUrls))
	}

	if len(issues) == 0 {
		issues = append(issues, "File validation passed - no issues found")
	}

	return issues
}
