package storage

import (
	"linkedin-crawler/internal/models"
)

// Legacy function wrappers for backward compatibility

var globalFileManager = NewFileManager()
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
func RemoveEmailFromFile(filePath string, emailToRemove string) error {
	return globalEmailStorage.RemoveEmailFromFile(filePath, emailToRemove)
}

// WriteEmailsToFile writes emails to file (legacy function)
func WriteEmailsToFile(filePath string, emails []string) error {
	return globalEmailStorage.WriteEmailsToFile(filePath, emails)
}

// RemoveAccountFromFile removes an account from file (legacy function)
func RemoveAccountFromFile(filePath string, acc models.Account) error {
	return globalAccountStorage.RemoveAccountFromFile(filePath, acc)
}

// LoadEmailsFromFile loads emails from file (legacy function)
func LoadEmailsFromFile(filePath string) ([]string, error) {
	return globalEmailStorage.LoadEmailsFromFile(filePath)
}

// LoadAccounts loads accounts from file (legacy function)
func LoadAccounts(filename string) ([]models.Account, error) {
	return globalAccountStorage.LoadAccounts(filename)
}
