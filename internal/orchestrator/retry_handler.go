package orchestrator

import (
	"fmt"
	"time"

	"linkedin-crawler/internal/crawler"
	"linkedin-crawler/internal/storage"
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

// RetryFailedEmails handles Phase 2 retry - processes failed emails from SQLite
func (rh *RetryHandler) RetryFailedEmails() error {
	maxRetry := 7
	emailStorage, tokenStorage, _ := rh.autoCrawler.GetStorageServices()

	for i := 1; i <= maxRetry; i++ {
		config := rh.autoCrawler.GetConfig()

		// Get failed emails from SQLite
		failedEmails, err := emailStorage.GetEmailsByStatus(storage.StatusFailed)
		if err != nil {
			return fmt.Errorf("không thể lấy failed emails từ database: %w", err)
		}

		// Also get pending emails (unprocessed emails)
		pendingEmails, err := emailStorage.GetPendingEmails()
		if err != nil {
			return fmt.Errorf("không thể lấy pending emails từ database: %w", err)
		}

		// Combine failed and pending emails for retry
		var retryEmails []string
		retryEmails = append(retryEmails, failedEmails...)
		retryEmails = append(retryEmails, pendingEmails...)

		if len(retryEmails) == 0 {
			fmt.Println("✅ Không còn emails nào cần retry")
			return nil
		}

		fmt.Printf("🔄 Phase 2 - Lần %d: Retry %d emails (Failed: %d, Pending: %d)...\n",
			i, len(retryEmails), len(failedEmails), len(pendingEmails))

		// Show current stats
		stats, err := emailStorage.GetEmailStats()
		if err == nil {
			fmt.Printf("📊 Stats hiện tại: Success: %d | Failed: %d | Pending: %d | HasInfo: %d | NoInfo: %d\n",
				stats["success"], stats["failed"], stats["pending"], stats["has_info"], stats["no_info"])
		}

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

		// Reset failed emails to pending status before retry
		if len(failedEmails) > 0 {
			fmt.Printf("🔄 Reset %d failed emails thành pending để retry...\n", len(failedEmails))
			for _, email := range failedEmails {
				if err := emailStorage.UpdateEmailStatus(email, storage.StatusPending, false, false); err != nil {
					fmt.Printf("⚠️ Không thể reset status cho email %s: %v\n", email, err)
				}
			}
		}

		// Initialize crawler for retry
		if err := batchProcessor.initializeCrawler(validTokens); err != nil {
			return fmt.Errorf("failed to initialize crawler for retry: %w", err)
		}

		// Record email count before retry
		emailsBefore := len(retryEmails)
		_, _ = batchProcessor.crawlWithCurrentTokensAndLicenseCheck(retryEmails)

		// Close crawler
		crawlerInstance := rh.autoCrawler.GetCrawler()
		if crawlerInstance != nil {
			crawler.Close(crawlerInstance) // Use function instead of method
			rh.autoCrawler.SetCrawler(nil)
		}

		// Check progress after retry
		pendingAfter, err := emailStorage.GetPendingEmails()
		if err != nil {
			fmt.Printf("⚠️ Không thể lấy pending emails sau retry: %v\n", err)
			continue
		}

		failedAfter, err := emailStorage.GetEmailsByStatus(storage.StatusFailed)
		if err != nil {
			fmt.Printf("⚠️ Không thể lấy failed emails sau retry: %v\n", err)
			continue
		}

		emailsAfter := len(pendingAfter) + len(failedAfter)

		if emailsAfter == 0 {
			fmt.Println("✅ Đã retry hết, không còn email nào cần retry nữa.")
			break
		}

		if emailsAfter >= emailsBefore {
			fmt.Println("⚠️ Không còn tiến triển trong retry, dừng")
			break
		}

		fmt.Printf("📊 Retry lần %d: %d -> %d emails còn lại (Pending: %d, Failed: %d)\n",
			i, emailsBefore, emailsAfter, len(pendingAfter), len(failedAfter))

		// Show updated stats
		statsAfter, err := emailStorage.GetEmailStats()
		if err == nil {
			fmt.Printf("📈 Stats sau retry: Success: %d | Failed: %d | Pending: %d | HasInfo: %d | NoInfo: %d\n",
				statsAfter["success"], statsAfter["failed"], statsAfter["pending"],
				statsAfter["has_info"], statsAfter["no_info"])
		}
	}

	return nil
}
