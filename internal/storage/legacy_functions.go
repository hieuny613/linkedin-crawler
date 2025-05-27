package storage

import (
	"linkedin-crawler/internal/models"
)

// Legacy function wrappers for backward compatibility with SQLite integration

// var globalFileManager = NewFileManager()
var globalEmailStorage = NewEmailStorage()
var globalTokenStorage = NewTokenStorage()
var globalAccountStorage = NewAccountStorage()

// LoadTokensFromFile loads tokens from file (legacy function)
func LoadTokensFromFile(filePath string) ([]string, error) {
	return globalTokenStorage.LoadTokensFromFile(filePath)
}

// RemoveTokenFromFile removes a token from file (legacy function)
func RemoveTokenFromFile(filePath string, tokenToRemove string) error {
	return globalTokenStorage.RemoveTokenFromFile(filePath, tokenToRemove)
}

// RemoveEmailFromFile removes an email from file (legacy function)
// Note: This is now deprecated with SQLite, use UpdateEmailStatus instead
func RemoveEmailFromFile(filePath string, emailToRemove string) error {
	return globalEmailStorage.RemoveEmailFromFile(filePath, emailToRemove)
}

// WriteEmailsToFile writes emails to file (legacy function)
// Note: This is now deprecated with SQLite, emails are managed in database
func WriteEmailsToFile(filePath string, emails []string) error {
	return globalEmailStorage.WriteEmailsToFile(filePath, emails)
}

// RemoveAccountFromFile removes an account from file (legacy function)
func RemoveAccountFromFile(filePath string, acc models.Account) error {
	return globalAccountStorage.RemoveAccountFromFile(filePath, acc)
}

// LoadEmailsFromFile loads emails from file and imports to SQLite (legacy function)
func LoadEmailsFromFile(filePath string) ([]string, error) {
	return globalEmailStorage.LoadEmailsFromFile(filePath)
}

// LoadAccounts loads accounts from file (legacy function)
func LoadAccounts(filename string) ([]models.Account, error) {
	return globalAccountStorage.LoadAccounts(filename)
}

// SQLite-specific functions (new functionality)

// UpdateEmailStatus updates email status in SQLite
func UpdateEmailStatus(email string, status EmailStatus, hasInfo, noInfo bool) error {
	return globalEmailStorage.UpdateEmailStatus(email, status, hasInfo, noInfo)
}

// GetPendingEmails returns pending emails from SQLite
func GetPendingEmails() ([]string, error) {
	return globalEmailStorage.GetPendingEmails()
}

// GetEmailsByStatus returns emails by status from SQLite
func GetEmailsByStatus(status EmailStatus) ([]string, error) {
	return globalEmailStorage.GetEmailsByStatus(status)
}

// GetEmailStats returns email statistics from SQLite
func GetEmailStats() (map[string]int, error) {
	return globalEmailStorage.GetEmailStats()
}

// ExportPendingEmailsToFile exports pending emails back to file
func ExportPendingEmailsToFile(filePath string) error {
	return globalEmailStorage.ExportPendingEmailsToFile(filePath)
}

// InitEmailDB initializes the SQLite database
func InitEmailDB() error {
	return globalEmailStorage.InitDB()
}

// CloseEmailDB closes the SQLite database connection
func CloseEmailDB() error {
	return globalEmailStorage.CloseDB()
}
