package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EmailStorage handles email file operations
type EmailStorage struct {
	fileManager *FileManager
}

// NewEmailStorage creates a new EmailStorage instance
func NewEmailStorage() *EmailStorage {
	return &EmailStorage{
		fileManager: NewFileManager(),
	}
}

// LoadEmailsFromFile loads emails from a file
func (es *EmailStorage) LoadEmailsFromFile(filePath string) ([]string, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory for emails file: %w", err)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Printf("Emails file not found at %s, creating empty file\n", filePath)
		if err := os.WriteFile(filePath, []byte("example@example.com\n"), 0644); err != nil {
			return nil, fmt.Errorf("failed to create emails file: %w", err)
		}
	}

	lines, err := es.fileManager.ReadLines(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read emails file: %w", err)
	}

	var emails []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		email := line

		if strings.Contains(line, ",") {
			parts := strings.SplitN(line, ",", 2)
			email = strings.TrimSpace(parts[1])
		}

		if email != "" && !strings.HasPrefix(email, "#") {
			emails = append(emails, email)
		}
	}

	return emails, nil
}

// WriteEmailsToFile writes emails to a file in a thread-safe manner
func (es *EmailStorage) WriteEmailsToFile(filePath string, emails []string) error {
	return es.fileManager.WriteLines(filePath, emails)
}

// RemoveEmailFromFile removes an email from a file in a thread-safe manner
func (es *EmailStorage) RemoveEmailFromFile(filePath string, emailToRemove string) error {
	return es.fileManager.RemoveLineFromFile(filePath, emailToRemove)
}
