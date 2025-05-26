package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	"linkedin-crawler/internal/models"
	"linkedin-crawler/internal/storage"
)

// TokenExtractor handles token extraction from browser
type TokenExtractor struct {
	loginService   *LoginService
	accountStorage *storage.AccountStorage
}

// NewTokenExtractor creates a new TokenExtractor instance
func NewTokenExtractor() *TokenExtractor {
	return &TokenExtractor{
		loginService:   NewLoginService(),
		accountStorage: storage.NewAccountStorage(),
	}
}

// GetTokenForAccount extracts LokiAuthToken for a given account
func (te *TokenExtractor) GetTokenForAccount(account models.Account, accountsFilePath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	browserManager := NewBrowserManager()
	browserCtx, browserCancel, err := browserManager.CreateBrowserContext(ctx)
	if err != nil {
		return "", err
	}
	defer browserCancel()

	// Perform login
	if err := te.loginService.LoginToTeams(browserCtx, account); err != nil {
		return "", fmt.Errorf("lỗi trong quá trình đăng nhập: %v", err)
	}

	// Extract token
	var lokiToken string
	err = chromedp.Evaluate(`sessionStorage.getItem("LokiAuthToken")`, &lokiToken).Do(browserCtx)
	if err != nil {
		return "", fmt.Errorf("lỗi khi lấy token: %v", err)
	}

	if lokiToken == "" {
		return "", fmt.Errorf("không lấy được LokiAuthToken")
	}

	cleanToken := strings.ReplaceAll(strings.ReplaceAll(lokiToken, "\"", ""), "\\", "")
	fmt.Printf("✅ Thành công lấy token cho: %s\n", account.Email)

	// Remove account from file after successful token extraction
	if rmErr := te.accountStorage.RemoveAccountFromFile(accountsFilePath, account); rmErr != nil {
		fmt.Printf("⚠️ Không thể xóa account %s: %v\n", account.Email, rmErr)
	} else {
		fmt.Printf("🗑️ Đã xóa account: %s\n", account.Email)
	}

	return cleanToken, nil
}

// ExtractTokensBatch extracts tokens from a batch of accounts
func (te *TokenExtractor) ExtractTokensBatch(accounts []models.Account, accountsFilePath string) []models.TokenResult {
	results := make(chan models.TokenResult, len(accounts))
	var wg sync.WaitGroup

	for _, account := range accounts {
		wg.Add(1)
		go func(acc models.Account) {
			defer wg.Done()
			token, err := te.GetTokenForAccount(acc, accountsFilePath)
			results <- models.TokenResult{
				Account: acc,
				Token:   token,
				Error:   err,
			}
		}(account)
	}

	wg.Wait()
	close(results)

	var tokenResults []models.TokenResult
	for result := range results {
		tokenResults = append(tokenResults, result)
	}

	return tokenResults
}
