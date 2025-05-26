package crawler

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"linkedin-crawler/internal/models"
	"linkedin-crawler/internal/storage"
)

// TokenManager handles token rotation and validation
type TokenManager struct {
	mutex sync.Mutex
}

// GetToken returns a random valid token
func (tm *TokenManager) GetToken(lc *models.LinkedInCrawler) string {
	lc.TokenMutex.Lock()
	defer lc.TokenMutex.Unlock()

	validTokens := []string{}
	for _, token := range lc.Tokens {
		if !lc.InvalidTokens[token] {
			validTokens = append(validTokens, token)
		}
	}

	if len(validTokens) == 0 {
		if len(lc.Tokens) > 0 {
			return lc.Tokens[0]
		}
		return ""
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	idx := r.Intn(len(validTokens))
	return validTokens[idx]
}

// AreAllTokensFailed checks if all tokens have failed
func (tm *TokenManager) AreAllTokensFailed(lc *models.LinkedInCrawler) bool {
	lc.TokenMutex.Lock()
	defer lc.TokenMutex.Unlock()
	return lc.AllTokensFailed
}

// MarkTokenAsInvalid marks a token as invalid
func (tm *TokenManager) MarkTokenAsInvalid(lc *models.LinkedInCrawler, token string) {
	lc.TokenMutex.Lock()
	defer lc.TokenMutex.Unlock()
	lc.InvalidTokens[token] = true
}

// SetAllTokensFailed sets the flag indicating all tokens have failed
func (tm *TokenManager) SetAllTokensFailed(lc *models.LinkedInCrawler, failed bool) {
	lc.TokenMutex.Lock()
	defer lc.TokenMutex.Unlock()
	lc.AllTokensFailed = failed
}

// GetValidTokenCount returns the number of valid tokens
func (tm *TokenManager) GetValidTokenCount(lc *models.LinkedInCrawler) int {
	lc.TokenMutex.Lock()
	defer lc.TokenMutex.Unlock()

	validCount := 0
	for _, token := range lc.Tokens {
		if !lc.InvalidTokens[token] {
			validCount++
		}
	}
	return validCount
}

// CheckIfAllTokensInvalid checks if all tokens are now invalid and updates the flag
func (tm *TokenManager) CheckIfAllTokensInvalid(lc *models.LinkedInCrawler) bool {
	lc.TokenMutex.Lock()
	defer lc.TokenMutex.Unlock()

	invalidCount := 0
	for _, token := range lc.Tokens {
		if lc.InvalidTokens[token] {
			invalidCount++
		}
	}

	if invalidCount >= len(lc.Tokens) {
		lc.AllTokensFailed = true
		return true
	}
	return false
}

// ValidatorService handles token validation operations
type ValidatorService struct {
	tokenStorage *storage.TokenStorage
}

// NewValidatorService creates a new ValidatorService instance
func NewValidatorService() *ValidatorService {
	return &ValidatorService{
		tokenStorage: storage.NewTokenStorage(),
	}
}

// HasValidTokens checks if there are valid tokens available in file
func (vs *ValidatorService) HasValidTokens(config models.Config, outputFile string, totalEmails []string) bool {
	existingTokens, err := vs.tokenStorage.LoadTokensFromFile(config.TokensFilePath)
	if err != nil || len(existingTokens) == 0 {
		return false
	}

	// Quick check on a few tokens
	tempCrawler, err := New(config, outputFile)
	if err != nil {
		return false
	}
	defer Close(tempCrawler) // Use function instead of method

	tempCrawler.Tokens = existingTokens
	tempCrawler.InvalidTokens = make(map[string]bool)

	testEmail := "test@example.com"
	if len(totalEmails) > 0 {
		testEmail = totalEmails[0]
	}

	validCount := 0
	checkLimit := 3 // Only check first 3 tokens to save time

	queryService := NewQueryService()

	for i, token := range existingTokens {
		if i >= checkLimit {
			break
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, _, statusCode, err := queryService.DoQueryProfile(tempCrawler, ctx, testEmail, token)
		cancel()

		if err == nil && (statusCode == 200 || statusCode == 429 || statusCode == 500) {
			validCount++
		}
	}

	// If at least 2/3 of the first tokens are valid, consider tokens available
	return validCount >= 2
}

// ValidateExistingTokens validates existing tokens from file
func (vs *ValidatorService) ValidateExistingTokens(tokens []string, config models.Config, outputFile string, totalEmails []string) ([]string, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("no tokens to validate")
	}

	tempCrawler, err := New(config, outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary crawler: %w", err)
	}
	defer Close(tempCrawler) // Use function instead of method

	tempCrawler.Tokens = tokens
	tempCrawler.InvalidTokens = make(map[string]bool)
	tempCrawler.TokensFilePath = config.TokensFilePath

	var validTokens []string
	testEmail := "test@example.com"
	if len(totalEmails) > 0 {
		testEmail = totalEmails[0]
	}

	fmt.Printf("🔍 Kiểm tra %d tokens với email test: %s\n", len(tokens), testEmail)

	queryService := NewQueryService()

	for i, token := range tokens {
		fmt.Printf("  🔑 Kiểm tra token %d/%d...\n", i+1, len(tokens))

		ctx, cancel := context.WithTimeout(context.Background(), config.RequestTimeout)
		_, _, statusCode, err := queryService.DoQueryProfile(tempCrawler, ctx, testEmail, token)
		cancel()

		if err == nil || statusCode == 429 || statusCode == 500 {
			validTokens = append(validTokens, token)
			fmt.Printf("  ✅ Token %d hợp lệ (status: %d)\n", i+1, statusCode)
		} else {
			fmt.Printf("  ❌ Token %d không hợp lệ (status: %d, error: %v)\n", i+1, statusCode, err)
			// Only remove token when 401 or 424, NOT when 500
			if statusCode == 401 || statusCode == 424 {
				if err := vs.tokenStorage.RemoveTokenFromFile(config.TokensFilePath, token); err != nil {
					fmt.Printf("  ⚠️ Không thể xóa token khỏi file: %v\n", err)
				} else {
					fmt.Printf("  🗑️ Đã xóa token không hợp lệ khỏi file\n")
				}
			}
		}

		time.Sleep(1 * time.Second)
	}

	fmt.Printf("✅ Kết quả kiểm tra: %d/%d tokens hợp lệ\n", len(validTokens), len(tokens))
	return validTokens, nil
}

// ValidateTokensBatch validates a batch of tokens immediately after extraction
func (vs *ValidatorService) ValidateTokensBatch(tokens []string, config models.Config, outputFile string, totalEmails []string) ([]string, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("no tokens to validate")
	}

	tempCrawler, err := New(config, outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary crawler: %w", err)
	}
	defer Close(tempCrawler) // Use function instead of method

	tempCrawler.Tokens = tokens
	tempCrawler.InvalidTokens = make(map[string]bool)
	tempCrawler.TokensFilePath = config.TokensFilePath

	var validTokens []string
	testEmail := "test@example.com"
	if len(totalEmails) > 0 {
		testEmail = totalEmails[0]
	}

	queryService := NewQueryService()

	for i, token := range tokens {
		fmt.Printf("    🔑 Kiểm tra token %d/%d...\n", i+1, len(tokens))

		ctx, cancel := context.WithTimeout(context.Background(), config.RequestTimeout)
		_, _, statusCode, err := queryService.DoQueryProfile(tempCrawler, ctx, testEmail, token)
		cancel()

		if err == nil || statusCode == 429 || statusCode == 500 {
			validTokens = append(validTokens, token)
			fmt.Printf("    ✅ Token %d hợp lệ (status: %d)\n", i+1, statusCode)
		} else {
			fmt.Printf("    ❌ Token %d không hợp lệ (status: %d, error: %v) - Bỏ qua\n", i+1, statusCode, err)
		}

		time.Sleep(1 * time.Second)
	}

	return validTokens, nil
}
