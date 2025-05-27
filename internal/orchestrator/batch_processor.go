package orchestrator

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"linkedin-crawler/internal/auth"
	"linkedin-crawler/internal/crawler"
	"linkedin-crawler/internal/models"
	"linkedin-crawler/internal/storage"
)

// BatchProcessor handles batch processing of emails
type BatchProcessor struct {
	autoCrawler      *AutoCrawler
	tokenExtractor   *auth.TokenExtractor
	queryService     *crawler.QueryService
	validatorService *crawler.ValidatorService
}

// NewBatchProcessor creates a new BatchProcessor instance
func NewBatchProcessor(ac *AutoCrawler) *BatchProcessor {
	return &BatchProcessor{
		autoCrawler:      ac,
		tokenExtractor:   auth.NewTokenExtractor(),
		queryService:     crawler.NewQueryService(),
		validatorService: crawler.NewValidatorService(),
	}
}

// ProcessAllEmails processes all emails with improved token rotation
func (bp *BatchProcessor) ProcessAllEmails() error {
	fmt.Println("🔄 Phase 1: Xử lý tất cả emails với token rotation...")

	stateManager := bp.autoCrawler.stateManager

	// Main loop - continue until no emails left or no accounts left
	for stateManager.HasEmailsToProcess() {
		if atomic.LoadInt32(bp.autoCrawler.GetShutdownRequested()) == 1 {
			fmt.Println("⚠️ Nhận tín hiệu dừng, thoát khỏi vòng lặp chính")
			break
		}

		// Display current status
		remaining := stateManager.CountRemainingEmails()
		fmt.Printf("\n" + strings.Repeat("─", 60) + "\n")
		fmt.Printf("🔑 CẦN TOKENS MỚI - Kiểm tra tokens hiện có...\n")
		bp.autoCrawler.PrintCurrentStats()
		fmt.Printf("📧 Còn lại: %d emails chưa xử lý\n", remaining)
		fmt.Printf("📂 Account index hiện tại: %d/%d\n", bp.autoCrawler.GetUsedAccountIndex(), len(bp.autoCrawler.GetAccounts()))

		// STEP 1: Check if there are available tokens
		var validTokens []string
		config := bp.autoCrawler.GetConfig()
		_, tokenStorage, _ := bp.autoCrawler.GetStorageServices()

		if bp.hasValidTokens() {
			fmt.Println("🔍 Phát hiện có tokens khả dụng, đang load và validate...")
			existingTokens, err := tokenStorage.LoadTokensFromFile(config.TokensFilePath)
			if err == nil && len(existingTokens) > 0 {
				fmt.Printf("📂 Tìm thấy %d tokens trong file, đang kiểm tra chi tiết...\n", len(existingTokens))
				validTokens, err = bp.validateExistingTokens(existingTokens)
				if err != nil {
					fmt.Printf("⚠️ Lỗi khi kiểm tra tokens: %v\n", err)
					validTokens = []string{}
				}
			}
		} else {
			fmt.Println("🔍 Không có tokens khả dụng trong file, cần lấy tokens mới")
		}

		// STEP 2: If not enough tokens, get more from accounts
		if len(validTokens) < config.MinTokens {
			fmt.Printf("📊 Có %d tokens hợp lệ, cần thêm %d tokens\n",
				len(validTokens), config.MinTokens-len(validTokens))

			// Check if there are accounts left
			if bp.autoCrawler.GetUsedAccountIndex() >= len(bp.autoCrawler.GetAccounts()) {
				fmt.Println("❌ Đã hết accounts để lấy tokens!")
				if len(validTokens) > 0 {
					fmt.Printf("🔋 Sử dụng %d tokens còn lại...\n", len(validTokens))
				} else {
					fmt.Println("💀 Không còn tokens nào, dừng chương trình")
					break
				}
			} else {
				fmt.Printf("🔄 Lấy thêm tokens từ accounts (còn %d accounts)\n",
					len(bp.autoCrawler.GetAccounts())-bp.autoCrawler.GetUsedAccountIndex())

				newTokens, err := bp.getTokensBatch()
				if err != nil {
					fmt.Printf("❌ Lỗi lấy tokens: %v\n", err)
					if len(validTokens) == 0 {
						break
					}
				} else {
					// Merge old and new tokens
					allTokens := append(validTokens, newTokens...)
					validTokens = allTokens

					// Save all tokens to file
					if err := tokenStorage.SaveTokensToFile(config.TokensFilePath, validTokens); err != nil {
						fmt.Printf("⚠️ Lỗi lưu tokens: %v\n", err)
					}
					fmt.Printf("✅ Tổng cộng có %d tokens để sử dụng\n", len(validTokens))
				}
			}
		} else {
			fmt.Printf("✅ Đủ tokens (%d) để tiếp tục crawling\n", len(validTokens))
		}

		// STEP 3: Crawl with current tokens
		if len(validTokens) > 0 {
			fmt.Printf("▶️ BẮT ĐẦU CRAWLING với %d tokens...\n", len(validTokens))
			fmt.Printf(strings.Repeat("─", 60) + "\n\n")

			if err := bp.processEmailsWithTokens(validTokens); err != nil {
				fmt.Printf("⚠️ Lỗi khi xử lý emails: %v\n", err)
			}

			// Check if need to get more tokens
			if stateManager.HasEmailsToProcess() {
				fmt.Println("🔄 Còn emails chưa xử lý, chuẩn bị lấy tokens mới...")
				time.Sleep(5 * time.Second) // Short break before getting new tokens
				continue
			}
		} else {
			fmt.Println("❌ Không có tokens nào khả dụng, dừng chương trình")
			break
		}

		// If no emails left, exit
		if !stateManager.HasEmailsToProcess() {
			fmt.Println("✅ Đã xử lý hết emails!")
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

// getTokensBatch gets a batch of tokens from accounts
func (bp *BatchProcessor) getTokensBatch() ([]string, error) {
	var validTokens []string
	config := bp.autoCrawler.GetConfig()
	accounts := bp.autoCrawler.GetAccounts()
	usedIndex := bp.autoCrawler.GetUsedAccountIndex()
	tokensNeeded := config.MaxTokens

	fmt.Printf("🎯 Mục tiêu: Lấy %d tokens mới\n", tokensNeeded)

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
	fmt.Printf("🔄 Sử dụng %d accounts (từ index %d đến %d) để lấy %d tokens\n",
		len(accountsBatch), usedIndex, endIndex-1, tokensNeeded)

	// Process in small batches to avoid overload
	batchSize := 5
	processedAccounts := 0

	for i := 0; i < len(accountsBatch) && len(validTokens) < tokensNeeded; i += batchSize {
		if atomic.LoadInt32(bp.autoCrawler.GetShutdownRequested()) == 1 {
			fmt.Println("⚠️ Nhận tín hiệu dừng trong quá trình lấy tokens")
			break
		}

		end := i + batchSize
		if end > len(accountsBatch) {
			end = len(accountsBatch)
		}

		batch := accountsBatch[i:end]
		fmt.Printf("  📦 Xử lý batch %d-%d (cần thêm %d tokens)...\n",
			i+1, end, tokensNeeded-len(validTokens))

		// Get tokens from this batch
		rawTokens := bp.processAccountsBatch(batch)
		processedAccounts += len(batch)

		// Validate tokens immediately
		if len(rawTokens) > 0 {
			fmt.Printf("  🔍 Kiểm tra %d tokens vừa lấy được...\n", len(rawTokens))
			validatedTokens, err := bp.validateTokensBatch(rawTokens)
			if err != nil {
				fmt.Printf("  ⚠️ Lỗi khi validate tokens: %v\n", err)
			} else {
				fmt.Printf("  ✅ Có %d/%d tokens hợp lệ từ batch này\n",
					len(validatedTokens), len(rawTokens))
				validTokens = append(validTokens, validatedTokens...)
			}
		}

		// Update index to not reuse processed accounts
		bp.autoCrawler.SetUsedAccountIndex(bp.autoCrawler.GetUsedAccountIndex() + len(batch))

		// Display progress
		fmt.Printf("  📊 Tiến độ: %d/%d tokens | Đã dùng %d/%d accounts\n",
			len(validTokens), tokensNeeded, bp.autoCrawler.GetUsedAccountIndex(), len(accounts))

		// If enough tokens, stop
		if len(validTokens) >= tokensNeeded {
			fmt.Printf("  🎉 Đã đủ %d tokens!\n", len(validTokens))
			break
		}

		// Rest between batches (except last batch)
		if end < len(accountsBatch) && len(validTokens) < tokensNeeded {
			fmt.Println("  ⏳ Chờ 10 giây trước batch tiếp theo...")
			time.Sleep(10 * time.Second)
		}
	}

	fmt.Printf("✅ Kết quả: Lấy được %d/%d tokens từ %d accounts\n",
		len(validTokens), tokensNeeded, processedAccounts)

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
		}
	}
	return validTokens
}

// processEmailsWithTokens processes emails with the given tokens
func (bp *BatchProcessor) processEmailsWithTokens(tokens []string) error {
	if err := bp.initializeCrawler(tokens); err != nil {
		return fmt.Errorf("failed to initialize crawler: %w", err)
	}
	defer func() {
		crawlerInstance := bp.autoCrawler.GetCrawler()
		if crawlerInstance != nil {
			crawler.Close(crawlerInstance) // Use function instead of method
			bp.autoCrawler.SetCrawler(nil)
		}
	}()

	// Get remaining emails from SQLite
	stateManager := bp.autoCrawler.stateManager
	remainingEmails := stateManager.GetRemainingEmails()

	if len(remainingEmails) == 0 {
		fmt.Println("✅ Không còn emails nào cần xử lý")
		return nil
	}

	fmt.Printf("🎯 Tiếp tục crawl %d emails còn lại với %d tokens...\n", len(remainingEmails), len(tokens))

	processedCount, err := bp.crawlWithCurrentTokens(remainingEmails)

	fmt.Printf("✅ Đã xử lý %d emails trong batch này\n", processedCount)
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

	fmt.Printf("✅ Crawler đã sẵn sàng với %d tokens\n", len(tokens))
	return nil
}

// crawlWithCurrentTokens crawls emails with current tokens
func (bp *BatchProcessor) crawlWithCurrentTokens(emails []string) (int, error) {
	if len(emails) == 0 {
		return 0, nil
	}

	totalOriginalEmails := len(bp.autoCrawler.GetTotalEmails())

	// Get stats from SQLite
	emailStorage, _, _ := bp.autoCrawler.GetStorageServices()
	stats, err := emailStorage.GetEmailStats()
	if err != nil {
		fmt.Printf("⚠️ Không thể lấy stats từ database: %v\n", err)
		stats = make(map[string]int)
	}

	alreadyProcessed := stats["success"]

	fmt.Printf("🎯 Bắt đầu crawl %d emails với tokens hiện tại...\n", len(emails))
	fmt.Printf("📊 Tiến độ tổng thể: Đã hoàn thành %d/%d emails (%.1f%%)\n",
		alreadyProcessed, totalOriginalEmails, float64(alreadyProcessed)*100/float64(totalOriginalEmails))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Reset stats cho batch này
	crawlerInstance := bp.autoCrawler.GetCrawler()
	if crawlerInstance != nil {
		atomic.StoreInt32(&crawlerInstance.Stats.Processed, 0)
		atomic.StoreInt32(&crawlerInstance.Stats.Success, 0)
		atomic.StoreInt32(&crawlerInstance.Stats.Failed, 0)
		atomic.StoreInt32(&crawlerInstance.Stats.TokenErrors, 0)
		crawlerInstance.AllTokensFailed = false
	}

	emailCh := make(chan string, 100)
	done := make(chan struct{})

	// Status ticker
	statusTicker := time.NewTicker(1 * time.Second)
	go func() {
		defer statusTicker.Stop()
		lastDisplay := ""
		isFirstDisplay := true

		for {
			select {
			case <-ctx.Done():
				if !isFirstDisplay {
					fmt.Fprintf(os.Stderr, "\r\033[A\033[K\033[K\r")
				}
				fmt.Println()
				return
			case <-statusTicker.C:
				// Check token status
				allTokensFailed := false
				validTokenCount := 0
				totalTokens := 0
				batchProcessed := int32(0)
				batchSuccess := int32(0)
				batchFailed := int32(0)
				activeReqs := int32(0)

				crawlerInstance := bp.autoCrawler.GetCrawler()
				if crawlerInstance != nil {
					allTokensFailed = crawlerInstance.AllTokensFailed
					batchProcessed = atomic.LoadInt32(&crawlerInstance.Stats.Processed)
					batchSuccess = atomic.LoadInt32(&crawlerInstance.Stats.Success)
					batchFailed = atomic.LoadInt32(&crawlerInstance.Stats.Failed)
					activeReqs = atomic.LoadInt32(&crawlerInstance.ActiveRequests)
					totalTokens = len(crawlerInstance.Tokens)

					// Count valid tokens
					for _, token := range crawlerInstance.Tokens {
						if !crawlerInstance.InvalidTokens[token] {
							validTokenCount++
						}
					}
				}

				// If tokens failed, stop crawling to get new tokens
				if allTokensFailed {
					fmt.Printf("\n❌ Tất cả tokens đã hết hiệu lực, cần lấy tokens mới\n")
					cancel() // Stop current crawling
					return
				}

				// Get current stats from SQLite
				currentStats, err := emailStorage.GetEmailStats()
				if err != nil {
					currentStats = make(map[string]int)
				}

				batchPercent := 0.0
				if len(emails) > 0 {
					batchPercent = float64(batchProcessed) * 100 / float64(len(emails))
				}

				totalPercent := float64(currentStats["success"]) * 100 / float64(totalOriginalEmails)

				// Progress bar
				barWidth := 25
				completedWidth := int(float64(barWidth) * batchPercent / 100)
				bar := "["
				for i := 0; i < barWidth; i++ {
					if i < completedWidth {
						bar += "█"
					} else if i == completedWidth && batchPercent > 0 && completedWidth < barWidth {
						bar += "▓"
					} else {
						bar += "░"
					}
				}
				bar += "]"

				line1 := fmt.Sprintf("🔄 Batch: %s %.1f%% (%d/%d) | Success: %d | Failed: %d | Active: %d | Tokens: %d/%d",
					bar, batchPercent, batchProcessed, len(emails), batchSuccess, batchFailed, activeReqs, validTokenCount, totalTokens)

				line2 := fmt.Sprintf("📊 Total: %.1f%% (%d/%d) | ✅Data: %d | 📭NoData: %d | ❌Failed: %d | ⏳Pending: %d",
					totalPercent, currentStats["success"], totalOriginalEmails,
					currentStats["has_info"], currentStats["no_info"], currentStats["failed"], currentStats["pending"])

				newDisplay := line1 + "\n" + line2

				if newDisplay != lastDisplay {
					if !isFirstDisplay {
						fmt.Fprintf(os.Stderr, "\r\033[A\033[K\033[K")
					}
					fmt.Fprintf(os.Stderr, "%s\n%s", line1, line2)
					lastDisplay = newDisplay
					isFirstDisplay = false
				}
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

	// Consumer goroutines
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

					// Check tokens before processing email
					crawlerInstance := bp.autoCrawler.GetCrawler()
					if crawlerInstance != nil {
						allTokensFailed := crawlerInstance.AllTokensFailed
						if allTokensFailed {
							fmt.Printf("\n❌ Tokens hết hiệu lực trong quá trình crawl, dừng worker\n")
							cancel()
							return
						}

						atomic.AddInt32(&crawlerInstance.Stats.Processed, 1)
						success := bp.retryEmailWithSQLite(email, 5)

						if !success {
							bp.autoCrawler.LogLine(fmt.Sprintf("💾 Email %s thất bại sau 5 lần retry - đánh dấu failed trong DB", email))
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
		statusTicker.Stop()
		fmt.Fprintf(os.Stderr, "\r\033[A\033[K\033[K\r")
		fmt.Println()

		processed := int32(0)
		success := int32(0)
		failed := int32(0)
		crawlerInstance := bp.autoCrawler.GetCrawler()
		if crawlerInstance != nil {
			processed = atomic.LoadInt32(&crawlerInstance.Stats.Processed)
			success = atomic.LoadInt32(&crawlerInstance.Stats.Success)
			failed = atomic.LoadInt32(&crawlerInstance.Stats.Failed)
		}

		fmt.Printf("✅ Hoàn thành batch: Processed: %d | Success: %d | Failed: %d\n", processed, success, failed)

		// Get final stats from SQLite
		finalStats, err := emailStorage.GetEmailStats()
		if err == nil {
			fmt.Printf("📊 Current totals: ✅Data: %d | 📭NoData: %d\n", finalStats["has_info"], finalStats["no_info"])
		}

		return int(processed), nil

	case <-ctx.Done():
		statusTicker.Stop()
		fmt.Fprintf(os.Stderr, "\r\033[A\033[K\033[K\r")
		fmt.Println()

		processed := int32(0)
		crawlerInstance := bp.autoCrawler.GetCrawler()
		if crawlerInstance != nil {
			processed = atomic.LoadInt32(&crawlerInstance.Stats.Processed)
		}

		if atomic.LoadInt32(bp.autoCrawler.GetShutdownRequested()) == 1 {
			fmt.Printf("⚠️ Crawling bị dừng do Ctrl+C: Đã xử lý %d emails\n", processed)
		} else {
			fmt.Printf("🔄 Crawling tạm dừng để lấy tokens mới: Đã xử lý %d emails\n", processed)
		}
		return int(processed), ctx.Err()
	}
}

// retryEmailWithSQLite retries email with SQLite integration
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
				bp.autoCrawler.LogLine(fmt.Sprintf("❌ Tất cả tokens đã bị lỗi, dừng retry cho email: %s", email))
				// Update status to failed in SQLite
				emailStorage.UpdateEmailStatus(email, storage.StatusFailed, false, false)
				return false
			}

			reqCtx, reqCancel := context.WithTimeout(context.Background(), config.RequestTimeout)
			hasProfile, body, statusCode, _ := bp.queryService.QueryProfileWithRetryLogic(crawlerInstance, reqCtx, email)
			reqCancel()

			// Log attempt
			snippet := ""
			if len(body) > 200 {
				snippet = string(body[:200]) + "..."
			} else {
				snippet = string(body)
			}

			bp.autoCrawler.LogLine(fmt.Sprintf("Retry %d/%d - Email: %s | Status: %d | Response: %s",
				attempt, maxRetries, email, statusCode, snippet))

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
							bp.autoCrawler.LogLine(fmt.Sprintf("⚠️ Không thể cập nhật status trong DB cho email %s: %v", email, err))
						}

						bp.autoCrawler.LogLine(fmt.Sprintf("✅ Email có thông tin LinkedIn: %s | User: %s | URL: %s",
							email, profile.User, profile.LinkedInURL))

						// Write to hit.txt file
						profileExtractor.WriteProfileToFile(crawlerInstance, email, profile)
						atomic.AddInt32(&crawlerInstance.Stats.Success, 1)
					} else {
						// NO LINKEDIN INFO (200 response but no useful data)
						err := emailStorage.UpdateEmailStatus(email, storage.StatusSuccess, false, true)
						if err != nil {
							bp.autoCrawler.LogLine(fmt.Sprintf("⚠️ Không thể cập nhật status trong DB cho email %s: %v", email, err))
						}

						bp.autoCrawler.LogLine(fmt.Sprintf("📭 Email không có thông tin LinkedIn: %s", email))
						atomic.AddInt32(&crawlerInstance.Stats.Success, 1)
					}
				} else {
					// NO LINKEDIN INFO
					err := emailStorage.UpdateEmailStatus(email, storage.StatusSuccess, false, true)
					if err != nil {
						bp.autoCrawler.LogLine(fmt.Sprintf("⚠️ Không thể cập nhật status trong DB cho email %s: %v", email, err))
					}

					bp.autoCrawler.LogLine(fmt.Sprintf("📭 Email không có thông tin LinkedIn: %s", email))
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

				bp.autoCrawler.LogLine(fmt.Sprintf("⏳ Chờ %dms trước khi retry lần %d cho email: %s", delayMs, attempt+1, email))
				time.Sleep(delay)
			}
		}
	}

	// After retrying maxRetries times and still not successful
	bp.autoCrawler.LogLine(fmt.Sprintf("❌ Email %s thất bại sau %d lần retry - Đánh dấu failed trong DB", email, maxRetries))

	// Update status to failed in SQLite
	emailStorage.UpdateEmailStatus(email, storage.StatusFailed, false, false)

	crawlerInstance = bp.autoCrawler.GetCrawler()
	if crawlerInstance != nil {
		atomic.AddInt32(&crawlerInstance.Stats.Failed, 1)
	}
	return false
}
