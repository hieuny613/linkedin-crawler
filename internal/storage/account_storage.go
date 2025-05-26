package storage

import (
	"fmt"
	"os"
	"strings"

	"linkedin-crawler/internal/models"
)

// AccountStorage handles account file operations
type AccountStorage struct {
	fileManager *FileManager
}

// NewAccountStorage creates a new AccountStorage instance
func NewAccountStorage() *AccountStorage {
	return &AccountStorage{
		fileManager: NewFileManager(),
	}
}

// LoadAccounts loads accounts from a file
func (as *AccountStorage) LoadAccounts(filename string) ([]models.Account, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		sampleContent := `# Format: email|password
# Ví dụ:
# user1@example.com|password123
# user2@example.com|mypassword456
example@domain.com|yourpassword`

		if err := os.WriteFile(filename, []byte(sampleContent), 0644); err != nil {
			return nil, fmt.Errorf("không thể tạo file mẫu: %v", err)
		}
		return nil, fmt.Errorf("đã tạo file mẫu %s, vui lòng thêm accounts và chạy lại", filename)
	}

	lines, err := as.fileManager.ReadLines(filename)
	if err != nil {
		return nil, fmt.Errorf("không thể mở file %s: %v", filename, err)
	}

	var accounts []models.Account
	lineNum := 0

	for _, line := range lines {
		lineNum++
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			fmt.Printf("Cảnh báo: Dòng %d có format không đúng (bỏ qua): %s\n", lineNum, line)
			continue
		}

		email := strings.TrimSpace(parts[0])
		password := strings.TrimSpace(parts[1])

		if email == "" || password == "" {
			fmt.Printf("Cảnh báo: Dòng %d có email hoặc password trống (bỏ qua): %s\n", lineNum, line)
			continue
		}

		accounts = append(accounts, models.Account{
			Email:    email,
			Password: password,
		})
	}

	return accounts, nil
}

// RemoveAccountFromFile removes a specific account from a file
func (as *AccountStorage) RemoveAccountFromFile(filePath string, acc models.Account) error {
	lines, err := as.fileManager.ReadLines(filePath)
	if err != nil {
		return err
	}

	var newLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			newLines = append(newLines, line)
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) == 2 {
			email := strings.TrimSpace(parts[0])
			password := strings.TrimSpace(parts[1])
			if email != acc.Email || password != acc.Password {
				newLines = append(newLines, line)
			}
		} else {
			newLines = append(newLines, line)
		}
	}

	return as.fileManager.WriteLines(filePath, newLines)
}
