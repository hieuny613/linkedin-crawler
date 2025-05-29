package orchestrator

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"linkedin-crawler/internal/auth"
	"linkedin-crawler/internal/crawler"
	"linkedin-crawler/internal/licensing"
	"linkedin-crawler/internal/models"
	"linkedin-crawler/internal/storage"
)

// BatchProcessor handles batch processing of emails with GUI logging and license checking
type BatchProcessor struct {
	autoCrawler      *AutoCrawler
	tokenExtractor   *auth.TokenExtractor
	queryService     *crawler.QueryService
	validatorService *crawler.ValidatorService
	licenseWrapper   *licensing.LicensedCrawlerWrapper // License wrapper for checking

	// GUI logging interface
	guiLogger GUILogger

	// License tracking
	processedEmailsCount int32 // Track số emails đã process thành công
	successEmailsCount   int32 // Track số emails thành công (có kết quả)
}

// GUILogger interface for sending logs to GUI
type GUILogger interface {
	LogInfo(message string)
	LogWarning(message string)
	LogError(message string)
	LogSuccess(message string)
	UpdateProgress(processed, total int, message string)
}

// NewBatchProcessor creates a new BatchProcessor instance
func NewBatchProcessor(ac *AutoCrawler) *BatchProcessor {
	return &BatchProcessor{
		autoCrawler:          ac,
		tokenExtractor:       auth.NewTokenExtractor(),
		queryService:         crawler.NewQueryService(),
		validatorService:     crawler.NewValidatorService(),
		licenseWrapper:       licensing.NewLicensedCrawlerWrapper(),
		processedEmailsCount: 0,
		successEmailsCount:   0,
	}
}

// SetGUILogger sets the GUI logger interface
func (bp *BatchProcessor) SetGUILogger(logger GUILogger) {
	bp.guiLogger = logger
}

// SetLicenseWrapper sets the license wrapper (for dependency injection)
func (bp *BatchProcessor) SetLicenseWrapper(wrapper *licensing.LicensedCrawlerWrapper) {
	bp.licenseWrapper = wrapper
}

// logInfo logs info message to GUI instead of console
func (bp *BatchProcessor) logInfo(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if bp.guiLogger != nil {
		bp.guiLogger.LogInfo(message)
	}
}

// logWarning logs warning message to GUI instead of console
func (bp *BatchProcessor) logWarning(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if bp.guiLogger != nil {
		bp.guiLogger.LogWarning(message)
	}
}

// logError logs error message to GUI instead of console
func (bp *BatchProcessor) logError(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if bp.guiLogger != nil {
		bp.guiLogger.LogError(message)
	}
}

// logSuccess logs success message to GUI instead of console
func (bp *BatchProcessor) logSuccess(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if bp.guiLogger != nil {
		bp.guiLogger.LogSuccess(message)
	}
}

// updateProgress updates progress in GUI
func (bp *BatchProcessor) updateProgress(processed, total int, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if bp.guiLogger != nil {
		bp.guiLogger.UpdateProgress(processed, total, message)
	}
}

// checkLicenseLimitsBeforeProcessing kiểm tra license trước khi process emails
func (bp *BatchProcessor) checkLicenseLimitsBeforeProcessing(emailsToProcess int) error {
	if bp.licenseWrapper == nil {
		return fmt.Errorf("license not initialized")
	}

	// Lấy thống kê hiện tại
	currentProcessed := atomic.LoadInt32(&bp.processedEmailsCount)

	// Tính tổng số emails sẽ được process
	totalWillProcess := int(currentProcessed) + emailsToProcess

	// Lấy account count
	accountCount := len(bp.autoCrawler.GetAccounts())

	// Check license limits
	err := bp.licenseWrapper.CheckCrawlingLimits(totalWillProcess, accountCount)
	if err != nil {
		bp.logError("License limit check failed: %v", err)
		return err
	}

	bp.logInfo("✅ License check passed: Will process %d emails (total: %d)", emailsToProcess, totalWillProcess)
	return nil
}

// checkLicenseLimitsDuringProcessing kiểm tra license trong quá trình process
func (bp *BatchProcessor) checkLicenseLimitsDuringProcessing() error {
	if bp.licenseWrapper == nil {
		return fmt.Errorf("license not initialized")
	}

	// Lấy số emails đã process thành công
	currentSuccess := atomic.LoadInt32(&bp.successEmailsCount)

	// Lấy license info để check limits
	info := bp.licenseWrapper.GetLicenseInfo()
	maxEmails, ok := info["max_emails"].(int)
	if !ok {
		// Không thể lấy limit info, cho phép tiếp tục
		return nil
	}

	// Nếu unlimited (-1), không cần check
	if maxEmails <= 0 {
		return nil
	}

	// Check nếu đã vượt quá limit
	if int(currentSuccess) >= maxEmails {
		return fmt.Errorf("email processing limit reached: %d/%d successful emails processed", currentSuccess, maxEmails)
	}

	// Cảnh báo khi gần đến limit
	if int(currentSuccess) >= maxEmails-10 {
		bp.logWarning("Approaching email limit: %d/%d emails processed", currentSuccess, maxEmails)
	}

	return nil
}

// ProcessAllEmails processes all emails with GUI logging and license checking
func (bp *BatchProcessor) ProcessAllEmails() error {
	bp.logInfo("🔄 Phase 1: Xử lý tất cả emails với token rotation và license checking...")

	stateManager := bp.autoCrawler.stateManager

	// Main loop - continue until no emails left or no accounts left
	for stateManager.HasEmailsToProcess() {
		if atomic.LoadInt32(bp.autoCrawler.GetShutdownRequested()) == 1 {
			bp.logWarning("⚠️ Nhận tín hiệu dừng, thoát khỏi vòng lặp chính")
			break
		}

		// Display current status
		remaining := stateManager.CountRemainingEmails()
		bp.logInfo("🔑 CẦN TOKENS MỚI - Kiểm tra tokens hiện có...")
		bp.autoCrawler.PrintCurrentStats()
		bp.logInfo("📧 Còn lại: %d emails chưa xử lý", remaining)
		bp.logInfo("📂 Account index hiện tại: %d/%d", bp.autoCrawler.GetUsedAccountIndex(), len(bp.autoCrawler.GetAccounts()))

		// STEP 1: Check if there are available tokens
		var validTokens []string
		config := bp.autoCrawler.GetConfig()
		_, tokenStorage, _ := bp.autoCrawler.GetStorageServices()

		if bp.hasValidTokens() {
			bp.logInfo("🔍 Phát hiện có tokens khả dụng, đang load và validate...")
			existingTokens, err := tokenStorage.LoadTokensFromFile(config.TokensFilePath)
			if err == nil && len(existingTokens) > 0 {
				bp.logInfo("📂 Tìm thấy %d tokens trong file, đang kiểm tra chi tiết...", len(existingTokens))
				validTokens, err = bp.validateExistingTokens(existingTokens)
				if err != nil {
					bp.logError("⚠️ Lỗi khi kiểm tra tokens: %v", err)
					validTokens = []string{}
				}
			}
		} else {
			bp.logInfo("🔍 Không có tokens khả dụng trong file, cần lấy tokens mới")
		}

		// STEP 2: If not enough tokens, get more from accounts
		if len(validTokens) < config.MinTokens {
			bp.logInfo("📊 Có %d tokens hợp lệ, cần thêm %d tokens", len(validTokens), config.MinTokens-len(validTokens))

			// Check if there are accounts left
			if bp.autoCrawler.GetUsedAccountIndex() >= len(bp.autoCrawler.GetAccounts()) {
				bp.logError("❌ Đã hết accounts để lấy tokens!")
				if len(validTokens) > 0 {
					bp.logWarning("🔋 Sử dụng %d tokens còn lại...", len(validTokens))
				} else {
					bp.logError("💀 Không còn tokens nào, dừng chương trình")
					break
				}
			} else {
				bp.logInfo("🔄 Lấy thêm tokens từ accounts (còn %d accounts)", len(bp.autoCrawler.GetAccounts())-bp.autoCrawler.GetUsedAccountIndex())

				newTokens, err := bp.getTokensBatch()
				if err != nil {
					bp.logError("❌ Lỗi lấy tokens: %v", err)
					if len(validTokens) == 0 {
						break
					}
				} else {
					// Merge old and new tokens
					allTokens := append(validTokens, newTokens...)
					validTokens = allTokens

					// Save all tokens to file
					if err := tokenStorage.SaveTokensToFile(config.TokensFilePath, validTokens); err != nil {
						bp.logError("⚠️ Lỗi lưu tokens: %v", err)
					}
					bp.logSuccess("✅ Tổng cộng có %d tokens để sử dụng", len(validTokens))
				}
			}
		} else {
			bp.logSuccess("✅ Đủ tokens (%d) để tiếp tục crawling", len(validTokens))
		}

		// STEP 3: Crawl with current tokens
		if len(validTokens) > 0 {
			bp.logInfo("▶️ BẮT ĐẦU CRAWLING với %d tokens...", len(validTokens))

			if err := bp.processEmailsWithTokens(validTokens); err != nil {
				bp.logError("⚠️ Lỗi khi xử lý emails: %v", err)
			}

			// Check if need to get more tokens
			if stateManager.HasEmailsToProcess() {
				bp.logInfo("🔄 Còn emails chưa xử lý, chuẩn bị lấy tokens mới...")
				time.Sleep(5 * time.Second) // Short break before getting new tokens
				continue
			}
		} else {
			bp.logError("❌ Không có tokens nào khả dụng, dừng chương trình")
			break
		}

		// If no emails left, exit
		if !stateManager.HasEmailsToProcess() {
			bp.logSuccess("✅ Đã xử lý hết emails!")
			break
		}
	}

	return nil
}

// hasValidTokens checks if there are valid tokens available
func (bp *BatchProcessor) hasValidTokens() bool {
	config := bp.autoCrawler.GetConfig()
	outputFile := bp.autoCrawler.GetOutputFile()
	totalEmails := bp.autoCrawler.GetTotalEmails()

	return bp.validatorService.HasValidTokens(config, outputFile, totalEmails)
}

// validateExistingTokens validates existing tokens from file
func (bp *BatchProcessor) validateExistingTokens(tokens []string) ([]string, error) {
	config := bp.autoCrawler.GetConfig()
	outputFile := bp.autoCrawler.GetOutputFile()
	totalEmails := bp.autoCrawler.GetTotalEmails()

	return bp.validatorService.ValidateExistingTokens(tokens, config, outputFile, totalEmails)
}

// validateTokensBatch validates a batch of tokens immediately after extraction
func (bp *BatchProcessor) validateTokensBatch(tokens []string) ([]string, error) {
	config := bp.autoCrawler.GetConfig()
	outputFile := bp.autoCrawler.GetOutputFile()
	totalEmails := bp.autoCrawler.GetTotalEmails()

	return bp.validatorService.ValidateTokensBatch(tokens, config, outputFile, totalEmails)
}

// getTokensBatch gets a batch of tokens from accounts with GUI progress
func (bp *BatchProcessor) getTokensBatch() ([]string, error) {
	var validTokens []string
	config := bp.autoCrawler.GetConfig()
	accounts := bp.autoCrawler.GetAccounts()
	usedIndex := bp.autoCrawler.GetUsedAccountIndex()
	tokensNeeded := config.MaxTokens

	bp.logInfo("🎯 Mục tiêu: Lấy %d tokens mới", tokensNeeded)

	if usedIndex >= len(accounts) {
		return validTokens, fmt.Errorf("no more accounts available (used: %d/%d)",
			usedIndex, len(accounts))
	}

	// Calculate needed accounts - usually need 2-3 accounts for 1 successful token
	accountsNeeded := tokensNeeded * 3 // Buffer because not every account will succeed

	endIndex := usedIndex + accountsNeeded
	if endIndex > len(accounts) {
		endIndex = len(accounts)
	}

	accountsBatch := accounts[usedIndex:endIndex]
	bp.logInfo("🔄 Sử dụng %d accounts (từ index %d đến %d) để lấy %d tokens", len(accountsBatch), usedIndex, endIndex-1, tokensNeeded)

	// Process in small batches to avoid overload
	batchSize := 5
	processedAccounts := 0

	for i := 0; i < len(accountsBatch) && len(validTokens) < tokensNeeded; i += batchSize {
		if atomic.LoadInt32(bp.autoCrawler.GetShutdownRequested()) == 1 {
			bp.logWarning("⚠️ Nhận tín hiệu dừng trong quá trình lấy tokens")
			break
		}

		end := i + batchSize
		if end > len(accountsBatch) {
			end = len(accountsBatch)
		}

		batch := accountsBatch[i:end]
		bp.logInfo("📦 Xử lý batch %d-%d (cần thêm %d tokens)...", i+1, end, tokensNeeded-len(validTokens))

		// Get tokens from this batch
		rawTokens := bp.processAccountsBatch(batch)
		processedAccounts += len(batch)

		// Validate tokens immediately
		if len(rawTokens) > 0 {
			bp.logInfo("🔍 Kiểm tra %d tokens vừa lấy được...", len(rawTokens))
			validatedTokens, err := bp.validateTokensBatch(rawTokens)
			if err != nil {
				bp.logError("⚠️ Lỗi khi validate tokens: %v", err)
			} else {
				bp.logSuccess("✅ Có %d/%d tokens hợp lệ từ batch này", len(validatedTokens), len(rawTokens))
				validTokens = append(validTokens, validatedTokens...)
			}
		}

		// Update index to not reuse processed accounts
		bp.autoCrawler.SetUsedAccountIndex(bp.autoCrawler.GetUsedAccountIndex() + len(batch))

		// Display progress
		bp.updateProgress(len(validTokens), tokensNeeded, "📊 Tiến độ: %d/%d tokens | Đã dùng %d/%d accounts", len(validTokens), tokensNeeded, bp.autoCrawler.GetUsedAccountIndex(), len(accounts))

		// If enough tokens, stop
		if len(validTokens) >= tokensNeeded {
			bp.logSuccess("🎉 Đã đủ %d tokens!", len(validTokens))
			break
		}

		// Rest between batches (except last batch)
		if end < len(accountsBatch) && len(validTokens) < tokensNeeded {
			bp.logInfo("⏳ Chờ 10 giây trước batch tiếp theo...")
			time.Sleep(10 * time.Second)
		}
	}

	bp.logSuccess("✅ Kết quả: Lấy được %d/%d tokens từ %d accounts", len(validTokens), tokensNeeded, processedAccounts)

	return validTokens, nil
}

// processAccountsBatch processes a batch of accounts to get tokens
func (bp *BatchProcessor) processAccountsBatch(accounts []models.Account) []string {
	config := bp.autoCrawler.GetConfig()
	results := bp.tokenExtractor.ExtractTokensBatch(accounts, config.AccountsFilePath)

	var validTokens []string
	for _, result := range results {
		if result.Error == nil && result.Token != "" {
			validTokens = append(validTokens, result.Token)
			bp.logSuccess("✅ Thành công lấy token từ account: %s", result.Account.Email)
		} else {
			bp.logError("❌ Lỗi account %s: %v", result.Account.Email, result.Error)
		}
	}
	return validTokens
}

// processEmailsWithTokens processes emails với license checking
func (bp *BatchProcessor) processEmailsWithTokens(tokens []string) error {
	// STEP 1: Check license trước khi bắt đầu
	stateManager := bp.autoCrawler.stateManager
	remainingEmails := stateManager.GetRemainingEmails()

	if len(remainingEmails) == 0 {
		bp.logInfo("✅ Không còn emails nào cần xử lý")
		return nil
	}

	// STEP 2: License check trước khi process
	if err := bp.checkLicenseLimitsBeforeProcessing(len(remainingEmails)); err != nil {
		bp.logError("❌ License limit exceeded before processing: %v", err)
		return err
	}

	// STEP 3: Initialize crawler
	if err := bp.initializeCrawler(tokens); err != nil {
		return fmt.Errorf("failed to initialize crawler: %w", err)
	}
	defer func() {
		crawlerInstance := bp.autoCrawler.GetCrawler()
		if crawlerInstance != nil {
			crawler.Close(crawlerInstance)
			bp.autoCrawler.SetCrawler(nil)
		}
	}()

	bp.logInfo("🎯 Tiếp tục crawl %d emails còn lại với %d tokens...", len(remainingEmails), len(tokens))

	// STEP 4: Process với license checking
	processedCount, err := bp.crawlWithCurrentTokensAndLicenseCheck(remainingEmails)

	bp.logSuccess("✅ Đã xử lý %d emails trong batch này", processedCount)
	return err
}

// initializeCrawler initializes the LinkedIn crawler with tokens
func (bp *BatchProcessor) initializeCrawler(tokens []string) error {
	config := bp.autoCrawler.GetConfig()
	outputFile := bp.autoCrawler.GetOutputFile()

	newCrawler, err := crawler.New(config, outputFile)
	if err != nil {
		return fmt.Errorf("failed to create crawler: %w", err)
	}

	newCrawler.Tokens = tokens
	newCrawler.InvalidTokens = make(map[string]bool)
	newCrawler.TokensFilePath = config.TokensFilePath
	newCrawler.RateLimitedEmails = []string{}

	bp.autoCrawler.SetCrawler(newCrawler)

	bp.logSuccess("✅ Crawler đã sẵn sàng với %d tokens", len(tokens))
	return nil
}

// crawlWithCurrentTokensAndLicenseCheck - Enhanced version với license checking
func (bp *BatchProcessor) crawlWithCurrentTokensAndLicenseCheck(emails []string) (int, error) {
	if len(emails) == 0 {
		return 0, nil
	}

	totalOriginalEmails := len(bp.autoCrawler.GetTotalEmails())
	emailStorage, _, _ := bp.autoCrawler.GetStorageServices()

	// Get initial stats
	stats, err := emailStorage.GetEmailStats()
	if err != nil {
		bp.logError("⚠️ Không thể lấy stats từ database: %v", err)
		stats = make(map[string]int)
	}

	alreadyProcessed := stats["success"]
	atomic.StoreInt32(&bp.processedEmailsCount, int32(alreadyProcessed))
	atomic.StoreInt32(&bp.successEmailsCount, int32(stats["has_info"]+stats["no_info"]))

	bp.logInfo("🎯 Bắt đầu crawl %d emails với license checking...", len(emails))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Reset crawler stats
	crawlerInstance := bp.autoCrawler.GetCrawler()
	if crawlerInstance != nil {
		atomic.StoreInt32(&crawlerInstance.Stats.Processed, 0)
		atomic.StoreInt32(&crawlerInstance.Stats.Success, 0)
		atomic.StoreInt32(&crawlerInstance.Stats.Failed, 0)
		crawlerInstance.AllTokensFailed = false
	}

	emailCh := make(chan string, 100)
	done := make(chan struct{})

	// License check ticker - Kiểm tra license định kỳ
	licenseCheckTicker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	go func() {
		defer licenseCheckTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-licenseCheckTicker.C:
				if err := bp.checkLicenseLimitsDuringProcessing(); err != nil {
					bp.logError("❌ License limit exceeded during processing: %v", err)
					cancel() // Stop crawling
					return
				}
			}
		}
	}()

	// Status ticker
	statusTicker := time.NewTicker(2 * time.Second)
	go func() {
		defer statusTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-statusTicker.C:
				bp.updateProgressWithLicenseInfo(ctx, emailStorage, totalOriginalEmails, len(emails))
			}
		}
	}()

	// Producer goroutine
	go func() {
		defer close(emailCh)
		for _, email := range emails {
			select {
			case <-ctx.Done():
				return
			case emailCh <- email:
			}
		}
	}()

	// Consumer goroutines với license checking
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		config := bp.autoCrawler.GetConfig()
		maxConcurrency := int(config.MaxConcurrency)

		for i := 0; i < maxConcurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for email := range emailCh {
					select {
					case <-ctx.Done():
						return
					default:
					}

					if atomic.LoadInt32(bp.autoCrawler.GetShutdownRequested()) == 1 {
						return
					}

					// LICENSE CHECK: Kiểm tra trước khi process từng email
					if err := bp.checkLicenseLimitsDuringProcessing(); err != nil {
						bp.logError("❌ License limit reached, stopping processing: %v", err)
						cancel()
						return
					}

					// Process email
					crawlerInstance := bp.autoCrawler.GetCrawler()
					if crawlerInstance != nil {
						if crawlerInstance.AllTokensFailed {
							bp.logError("❌ Tokens hết hiệu lực, dừng worker")
							cancel()
							return
						}

						atomic.AddInt32(&crawlerInstance.Stats.Processed, 1)
						atomic.AddInt32(&bp.processedEmailsCount, 1)

						success := bp.retryEmailWithLicenseCheck(email, 5)
						if success {
							atomic.AddInt32(&bp.successEmailsCount, 1)
						}
					}
				}
			}()
		}
		wg.Wait()
	}()

	// Wait for completion
	select {
	case <-done:
		licenseCheckTicker.Stop()
		statusTicker.Stop()

		processed := int32(0)
		success := int32(0)
		failed := int32(0)
		crawlerInstance := bp.autoCrawler.GetCrawler()
		if crawlerInstance != nil {
			processed = atomic.LoadInt32(&crawlerInstance.Stats.Processed)
			success = atomic.LoadInt32(&crawlerInstance.Stats.Success)
			failed = atomic.LoadInt32(&crawlerInstance.Stats.Failed)
		}

		bp.logSuccess("✅ Hoàn thành batch: Processed: %d | Success: %d | Failed: %d", processed, success, failed)

		// Final license check
		finalErr := bp.checkLicenseLimitsDuringProcessing()
		if finalErr != nil {
			bp.logWarning("⚠️ License limit reached at end of batch: %v", finalErr)
		}

		return int(processed), nil

	case <-ctx.Done():
		licenseCheckTicker.Stop()
		statusTicker.Stop()

		processed := int32(0)
		crawlerInstance := bp.autoCrawler.GetCrawler()
		if crawlerInstance != nil {
			processed = atomic.LoadInt32(&crawlerInstance.Stats.Processed)
		}

		if atomic.LoadInt32(bp.autoCrawler.GetShutdownRequested()) == 1 {
			bp.logWarning("⚠️ Crawling stopped by user: Processed %d emails", processed)
		} else {
			bp.logInfo("🔄 Crawling stopped by license limit or tokens: Processed %d emails", processed)
		}
		return int(processed), ctx.Err()
	}
}

// updateProgressWithLicenseInfo cập nhật progress với thông tin license
func (bp *BatchProcessor) updateProgressWithLicenseInfo(ctx context.Context, emailStorage *storage.EmailStorage, totalOriginalEmails, currentBatchSize int) {
	// Get current stats
	currentStats, err := emailStorage.GetEmailStats()
	if err != nil {
		currentStats = make(map[string]int)
	}

	// Get license info
	licenseInfo := ""
	if bp.licenseWrapper != nil {
		info := bp.licenseWrapper.GetLicenseInfo()
		if maxEmails, ok := info["max_emails"].(int); ok && maxEmails > 0 {
			successCount := currentStats["success"]
			licenseInfo = fmt.Sprintf(" | License: %d/%d", successCount, maxEmails)
		} else {
			licenseInfo = " | License: Unlimited"
		}
	}

	batchPercent := 0.0
	if currentBatchSize > 0 {
		batchProcessed := atomic.LoadInt32(&bp.processedEmailsCount)
		batchPercent = float64(batchProcessed) * 100 / float64(currentBatchSize)
	}

	totalPercent := float64(currentStats["success"]) * 100 / float64(totalOriginalEmails)

	bp.updateProgress(int(atomic.LoadInt32(&bp.processedEmailsCount)), currentBatchSize,
		"🔄 Batch: %.1f%% | Total: %.1f%% | Success: %d | Failed: %d%s",
		batchPercent, totalPercent, currentStats["success"], currentStats["failed"], licenseInfo)
}

// retryEmailWithLicenseCheck - Enhanced retry với license checking
func (bp *BatchProcessor) retryEmailWithLicenseCheck(email string, maxRetries int) bool {
	// License check trước khi retry
	if err := bp.checkLicenseLimitsDuringProcessing(); err != nil {
		bp.logError("❌ License limit reached, skipping email: %s (%v)", email, err)
		return false
	}

	// Proceed với regular retry logic
	return bp.retryEmailWithSQLite(email, maxRetries)
}

// retryEmailWithSQLite retries email with SQLite integration - GUI LOGGING
func (bp *BatchProcessor) retryEmailWithSQLite(email string, maxRetries int) bool {
	config := bp.autoCrawler.GetConfig()
	crawlerInstance := bp.autoCrawler.GetCrawler()
	emailStorage, _, _ := bp.autoCrawler.GetStorageServices()

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if atomic.LoadInt32(bp.autoCrawler.GetShutdownRequested()) == 1 {
			return false
		}

		if crawlerInstance != nil {
			allTokensFailed := crawlerInstance.AllTokensFailed
			if allTokensFailed {
				bp.logError("❌ Tất cả tokens đã bị lỗi, dừng retry cho email: %s", email)
				emailStorage.UpdateEmailStatus(email, storage.StatusFailed, false, false)
				return false
			}

			reqCtx, reqCancel := context.WithTimeout(context.Background(), config.RequestTimeout)
			hasProfile, body, statusCode, _ := bp.queryService.QueryProfileWithRetryLogic(crawlerInstance, reqCtx, email)
			reqCancel()

			// Only log detailed info on final attempt or success
			if attempt == maxRetries || statusCode == 200 {
				bp.logInfo("Retry %d/%d - Email: %s | Status: %d", attempt, maxRetries, email, statusCode)
			}

			// Process successful response
			if statusCode == 200 {
				if hasProfile {
					// Check if there's actual profile data
					profileExtractor := crawler.NewProfileExtractor()
					profile, parseErr := profileExtractor.ExtractProfileData(body)
					if parseErr == nil && profile.User != "" && profile.User != "null" && profile.User != "{}" {
						// HAS LINKEDIN INFO
						err := emailStorage.UpdateEmailStatus(email, storage.StatusSuccess, true, false)
						if err != nil {
							bp.logError("⚠️ Không thể cập nhật status trong DB cho email %s: %v", email, err)
						}

						bp.logSuccess("✅ Email có thông tin LinkedIn: %s | User: %s", email, profile.User)

						// Write to hit.txt file
						profileExtractor.WriteProfileToFile(crawlerInstance, email, profile)
						atomic.AddInt32(&crawlerInstance.Stats.Success, 1)
					} else {
						// NO LINKEDIN INFO (200 response but no useful data)
						err := emailStorage.UpdateEmailStatus(email, storage.StatusSuccess, false, true)
						if err != nil {
							bp.logError("⚠️ Không thể cập nhật status trong DB cho email %s: %v", email, err)
						}

						bp.logInfo("📭 Email không có thông tin LinkedIn: %s", email)
						atomic.AddInt32(&crawlerInstance.Stats.Success, 1)
					}
				} else {
					// NO LINKEDIN INFO
					err := emailStorage.UpdateEmailStatus(email, storage.StatusSuccess, false, true)
					if err != nil {
						bp.logError("⚠️ Không thể cập nhật status trong DB cho email %s: %v", email, err)
					}

					bp.logInfo("📭 Email không có thông tin LinkedIn: %s", email)
					atomic.AddInt32(&crawlerInstance.Stats.Success, 1)
				}

				return true
			}

			// If not last attempt and not successful, wait before retry
			if attempt < maxRetries {
				// Random delay between 200-600ms
				r := rand.New(rand.NewSource(time.Now().UnixNano()))
				delayMs := 200 + r.Intn(401) // 200 + (0-400) = 200-600ms
				delay := time.Duration(delayMs) * time.Millisecond
				time.Sleep(delay)
			}
		}
	}

	// After retrying maxRetries times and still not successful
	bp.logError("❌ Email %s thất bại sau %d lần retry - Đánh dấu failed trong DB", email, maxRetries)

	// Update status to failed in SQLite
	emailStorage.UpdateEmailStatus(email, storage.StatusFailed, false, false)

	crawlerInstance = bp.autoCrawler.GetCrawler()
	if crawlerInstance != nil {
		atomic.AddInt32(&crawlerInstance.Stats.Failed, 1)
	}
	return false
}

// GetLicenseStats returns current license usage statistics
func (bp *BatchProcessor) GetLicenseStats() map[string]interface{} {
	if bp.licenseWrapper == nil {
		return map[string]interface{}{
			"license_active": false,
			"error":          "license not initialized",
		}
	}

	info := bp.licenseWrapper.GetLicenseInfo()
	currentProcessed := atomic.LoadInt32(&bp.processedEmailsCount)
	currentSuccess := atomic.LoadInt32(&bp.successEmailsCount)

	stats := map[string]interface{}{
		"license_active":    true,
		"current_processed": int(currentProcessed),
		"current_success":   int(currentSuccess),
		"license_info":      info,
	}

	return stats
}

// UpdateLicenseUsage updates license usage counters (called externally)
func (bp *BatchProcessor) UpdateLicenseUsage(processed, success int) {
	atomic.StoreInt32(&bp.processedEmailsCount, int32(processed))
	atomic.StoreInt32(&bp.successEmailsCount, int32(success))

	// Also update license wrapper
	if bp.licenseWrapper != nil {
		bp.licenseWrapper.UpdateUsageCounters(processed, success)
	}
}

// ResetLicenseCounters resets license usage counters
func (bp *BatchProcessor) ResetLicenseCounters() {
	atomic.StoreInt32(&bp.processedEmailsCount, 0)
	atomic.StoreInt32(&bp.successEmailsCount, 0)

	if bp.licenseWrapper != nil {
		bp.licenseWrapper.ResetUsageCounters()
	}
}

// GetCurrentUsage returns current usage statistics
func (bp *BatchProcessor) GetCurrentUsage() (processed, success int) {
	return int(atomic.LoadInt32(&bp.processedEmailsCount)), int(atomic.LoadInt32(&bp.successEmailsCount))
}

// IsLicenseValid checks if license is currently valid
func (bp *BatchProcessor) IsLicenseValid() bool {
	if bp.licenseWrapper == nil {
		return false
	}

	err := bp.licenseWrapper.ValidateAndStart()
	return err == nil
}

// GetRemainingEmailQuota returns remaining email quota based on license
func (bp *BatchProcessor) GetRemainingEmailQuota() int {
	if bp.licenseWrapper == nil {
		return 0
	}

	info := bp.licenseWrapper.GetLicenseInfo()
	maxEmails, ok := info["max_emails"].(int)
	if !ok || maxEmails <= 0 {
		return -1 // Unlimited
	}

	currentProcessed := atomic.LoadInt32(&bp.processedEmailsCount)
	remaining := maxEmails - int(currentProcessed)
	if remaining < 0 {
		remaining = 0
	}

	return remaining
}

// ShowLicenseStatus displays current license status (for debugging)
func (bp *BatchProcessor) ShowLicenseStatus() {
	if bp.licenseWrapper != nil {
		bp.licenseWrapper.ShowLicenseStatus()
	} else {
		bp.logError("License wrapper not initialized")
	}
}
