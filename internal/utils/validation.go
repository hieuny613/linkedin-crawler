package utils

import (
	"fmt"
	"regexp"
	"strings"
)

// Email validation regex pattern
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// Token validation regex pattern
var tokenRegex = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// IsValidEmail validates email format
func IsValidEmail(email string) bool {
	email = strings.TrimSpace(email)
	return emailRegex.MatchString(email)
}

// IsValidTokenFormat validates token format
// LinkedIn tokens should be long alphanumeric strings with dots, underscores, and hyphens
func IsValidTokenFormat(token string) bool {
	token = strings.TrimSpace(token)
	return len(token) > 50 && tokenRegex.MatchString(token)
}

// IsValidPassword validates password (minimum 6 characters)
func IsValidPassword(password string) bool {
	password = strings.TrimSpace(password)
	return len(password) >= 6
}

// ExtractEmailsFromLine extracts all valid emails from a line (handles comma separation)
func ExtractEmailsFromLine(line string) []string {
	var emails []string
	line = strings.TrimSpace(line)

	if line == "" || strings.HasPrefix(line, "#") {
		return emails
	}

	// Handle multiple emails per line (comma-separated)
	if strings.Contains(line, ",") {
		parts := strings.Split(line, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if IsValidEmail(part) {
				emails = append(emails, part)
			}
		}
	} else {
		// Single email per line
		if IsValidEmail(line) {
			emails = append(emails, line)
		}
	}

	return emails
}

// ValidateTokenBatch validates a batch of tokens and returns counts
func ValidateTokenBatch(tokens []string) (validCount, invalidCount int) {
	for _, token := range tokens {
		if IsValidTokenFormat(token) {
			validCount++
		} else {
			invalidCount++
		}
	}
	return validCount, invalidCount
}

// RemoveDuplicateEmails removes duplicate emails from a slice while preserving order
func RemoveDuplicateEmails(emails []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, email := range emails {
		email = strings.ToLower(strings.TrimSpace(email)) // Normalize
		if !seen[email] {
			seen[email] = true
			result = append(result, email)
		}
	}

	return result
}

// NormalizeEmail converts email to lowercase and trims whitespace
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// CleanToken removes Bearer prefix and trims whitespace from token
func CleanToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	return token
}

// ValidationResult represents the result of validating a batch of items
type ValidationResult struct {
	Valid    int      `json:"valid"`
	Invalid  int      `json:"invalid"`
	Total    int      `json:"total"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// ValidateEmailBatch validates a batch of emails and returns detailed results
func ValidateEmailBatch(emails []string) ValidationResult {
	result := ValidationResult{
		Total:    len(emails),
		Errors:   []string{},
		Warnings: []string{},
	}

	seen := make(map[string]bool)
	duplicateCount := 0

	for i, email := range emails {
		email = strings.TrimSpace(email)

		if email == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("Line %d: Empty email", i+1))
			result.Invalid++
			continue
		}

		// Check for duplicates
		normalizedEmail := NormalizeEmail(email)
		if seen[normalizedEmail] {
			duplicateCount++
			result.Warnings = append(result.Warnings, fmt.Sprintf("Line %d: Duplicate email %s", i+1, email))
			result.Invalid++
			continue
		}
		seen[normalizedEmail] = true

		// Validate format
		if IsValidEmail(email) {
			result.Valid++
		} else {
			result.Errors = append(result.Errors, fmt.Sprintf("Line %d: Invalid email format: %s", i+1, email))
			result.Invalid++
		}
	}

	if duplicateCount > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Found %d duplicate emails", duplicateCount))
	}

	return result
}

// ValidateAccountBatch validates a batch of accounts and returns detailed results
func ValidateAccountBatch(accounts []string) ValidationResult {
	result := ValidationResult{
		Total:  len(accounts),
		Errors: []string{},
	}

	for i, line := range accounts {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			result.Errors = append(result.Errors, fmt.Sprintf("Line %d: Invalid format, expected email|password", i+1))
			result.Invalid++
			continue
		}

		email := strings.TrimSpace(parts[0])
		password := strings.TrimSpace(parts[1])

		if !IsValidEmail(email) {
			result.Errors = append(result.Errors, fmt.Sprintf("Line %d: Invalid email format: %s", i+1, email))
			result.Invalid++
			continue
		}

		if !IsValidPassword(password) {
			result.Errors = append(result.Errors, fmt.Sprintf("Line %d: Password too short (minimum 6 characters)", i+1))
			result.Invalid++
			continue
		}

		result.Valid++
	}

	return result
}
