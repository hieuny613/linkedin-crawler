package storage

import (
	"fmt"
	"strings"
)

// TokenStorage handles token file operations
type TokenStorage struct {
	fileManager *FileManager
}

// NewTokenStorage creates a new TokenStorage instance
func NewTokenStorage() *TokenStorage {
	return &TokenStorage{
		fileManager: NewFileManager(),
	}
}

// LoadTokensFromFile loads tokens from a file
func (ts *TokenStorage) LoadTokensFromFile(filePath string) ([]string, error) {
	lines, err := ts.fileManager.ReadLines(filePath)
	if err != nil {
		return nil, fmt.Errorf("tokens file does not exist or cannot be read: %w", err)
	}

	var tokens []string
	for _, line := range lines {
		token := strings.TrimSpace(line)
		if token != "" {
			token = strings.TrimPrefix(token, "Bearer ")
			tokens = append(tokens, token)
		}
	}

	return tokens, nil
}

// SaveTokensToFile saves tokens to a file, merging with existing tokens
func (ts *TokenStorage) SaveTokensToFile(filePath string, tokens []string) error {
	existingTokens, _ := ts.LoadTokensFromFile(filePath)

	tokenMap := make(map[string]bool)
	for _, token := range existingTokens {
		tokenMap[token] = true
	}

	for _, token := range tokens {
		tokenMap[token] = true
	}

	var allTokens []string
	for token := range tokenMap {
		allTokens = append(allTokens, token)
	}

	if err := ts.fileManager.WriteLines(filePath, allTokens); err != nil {
		return fmt.Errorf("failed to save tokens: %w", err)
	}

	fmt.Printf("💾 Đã lưu %d tokens vào %s\n", len(allTokens), filePath)
	return nil
}

// RemoveTokenFromFile removes a specific token from a file
func (ts *TokenStorage) RemoveTokenFromFile(filePath string, tokenToRemove string) error {
	return ts.fileManager.RemoveLineFromFile(filePath, tokenToRemove)
}
