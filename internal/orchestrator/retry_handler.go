package orchestrator

import (
	"fmt"
	"time"

	"linkedin-crawler/internal/crawler"
)

// RetryHandler handles retry logic for failed emails
type RetryHandler struct {
	autoCrawler *AutoCrawler
}

// NewRetryHandler creates a new RetryHandler instance
func NewRetryHandler(ac *AutoCrawler) *RetryHandler {
	return &RetryHandler{
		autoCrawler: ac,
	}
}

// RetryFailedEmails handles Phase 2 retry - processes ALL remaining emails from file
func (rh *RetryHandler) RetryFailedEmails() error {
	maxRetry := 7

	for i := 1; i <= maxRetry; i++ {
		emailStorage, tokenStorage, _ := rh.autoCrawler.GetStorageServices()
		config := rh.autoCrawler.GetConfig()

		// Read remaining emails from file (includes both unprocessed and failed)
		retryEmails, err := emailStorage.LoadEmailsFromFile(config.EmailsFilePath)
		if err != nil || len(retryEmails) == 0 {
			fmt.Println("✅ Không còn emails nào cần retry")
			return nil
		}

		fmt.Printf("🔄 Phase 2 - Lần %d: Retry %d emails còn lại...\n", i, len(retryEmails))
		fmt.Println("⏳ Chờ 10 giây trước khi retry...")
		time.Sleep(10 * time.Second)

		// Get tokens for retry
		existingTokens, err := tokenStorage.LoadTokensFromFile(config.TokensFilePath)
		if err != nil || len(existingTokens) == 0 {
			fmt.Println("🔑 Không có tokens, lấy tokens mới cho retry...")
			if rh.autoCrawler.GetUsedAccountIndex() < len(rh.autoCrawler.GetAccounts()) {
				batchProcessor := rh.autoCrawler.batchProcessor
				tokens, err := batchProcessor.getTokensBatch()
				if err != nil {
					return fmt.Errorf("không thể lấy tokens cho retry: %w", err)
				}
				existingTokens = tokens
			} else {
				fmt.Println("⚠️ Không còn accounts để lấy tokens cho retry")
				return nil
			}
		}

		batchProcessor := rh.autoCrawler.batchProcessor
		validTokens, err := batchProcessor.validateExistingTokens(existingTokens)
		if err != nil {
			return fmt.Errorf("lỗi validate tokens cho retry: %w", err)
		}

		if len(validTokens) == 0 {
			fmt.Println("❌ Không có tokens hợp lệ cho retry")
			return nil
		}

		fmt.Printf("🔄 Retry với %d tokens hợp lệ...\n", len(validTokens))

		// Initialize crawler for retry
		if err := batchProcessor.initializeCrawler(validTokens); err != nil {
			return fmt.Errorf("failed to initialize crawler for retry: %w", err)
		}

		// Record email count before retry
		emailsBefore := len(retryEmails)
		_, _ = batchProcessor.crawlWithCurrentTokens(retryEmails)

		// Close crawler
		crawlerInstance := rh.autoCrawler.GetCrawler()
		if crawlerInstance != nil {
			crawler.Close(crawlerInstance) // Use function instead of method
			rh.autoCrawler.SetCrawler(nil)
		}

		// Check after retry
		emailsAfterList, _ := emailStorage.LoadEmailsFromFile(config.EmailsFilePath)
		emailsAfter := len(emailsAfterList)

		if emailsAfter == 0 {
			fmt.Println("✅ Đã retry hết, không còn email nào cần retry nữa.")
			break
		}

		if emailsAfter >= emailsBefore {
			fmt.Println("⚠️ Không còn tiến triển trong retry, dừng")
			break
		}

		fmt.Printf("📊 Retry lần %d: %d -> %d emails còn lại\n", i, emailsBefore, emailsAfter)
	}
	return nil
}
