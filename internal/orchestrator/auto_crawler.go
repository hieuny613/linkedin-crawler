package orchestrator

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"linkedin-crawler/internal/models"
	"linkedin-crawler/internal/storage"
	"linkedin-crawler/internal/utils"
)

// AutoCrawler orchestrates the LinkedIn crawling process with SQLite integration
type AutoCrawler struct {
	config            models.Config
	accounts          []models.Account
	usedAccountIndex  int
	crawler           *models.LinkedInCrawler
	crawlerMutex      sync.RWMutex
	outputFile        string
	totalEmails       []string
	processedEmails   int
	shutdownRequested int32

	logFile      *os.File
	logWriter    *bufio.Writer
	logChan      chan string
	logWaitGroup sync.WaitGroup

	// File operation mutex để tránh race condition
	fileOpMutex sync.Mutex

	// Storage services
	emailStorage   *storage.EmailStorage
	tokenStorage   *storage.TokenStorage
	accountStorage *storage.AccountStorage

	// Processing services
	batchProcessor *BatchProcessor
	retryHandler   *RetryHandler
	stateManager   *StateManager

	// Database cleanup flag
	dbCleanupDone int32
}

// New creates a new AutoCrawler instance with SQLite integration
func New(config models.Config) (*AutoCrawler, error) {
	outputFile := "hit.txt"

	// Initialize storage services
	emailStorage := storage.NewEmailStorage()
	tokenStorage := storage.NewTokenStorage()
	accountStorage := storage.NewAccountStorage()

	// Load accounts
	accounts, err := accountStorage.LoadAccounts(config.AccountsFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load accounts: %w", err)
	}

	// Load emails and import to SQLite (with validation and deduplication)
	emails, err := emailStorage.LoadEmailsFromFile(config.EmailsFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load emails: %w", err)
	}

	// Setup logging
	logFile, err := os.OpenFile("crawler.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	ac := &AutoCrawler{
		config:           config,
		accounts:         accounts,
		usedAccountIndex: 0,
		outputFile:       outputFile,
		totalEmails:      emails,
		processedEmails:  0,
		logFile:          logFile,
		logWriter:        bufio.NewWriter(logFile),
		logChan:          make(chan string, 1000),
		dbCleanupDone:    0,

		// Initialize storage services
		emailStorage:   emailStorage,
		tokenStorage:   tokenStorage,
		accountStorage: accountStorage,
	}

	// Initialize processing services
	ac.batchProcessor = NewBatchProcessor(ac)
	ac.retryHandler = NewRetryHandler(ac)
	ac.stateManager = NewStateManager(ac)

	// Start logging goroutine
	ac.logWaitGroup.Add(1)
	go func() {
		defer ac.logWaitGroup.Done()
		for line := range ac.logChan {
			_, err := ac.logWriter.WriteString(line + "\n")
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️ Lỗi ghi log: %v\n", err)
			}
		}
		ac.logWriter.Flush()
		ac.logFile.Close()
	}()

	// Setup signal handling
	utils.SetupSignalHandling(&ac.shutdownRequested, ac.gracefulShutdown, config.SleepDuration)

	return ac, nil
}

// gracefulShutdown handles graceful shutdown including database cleanup
func (ac *AutoCrawler) gracefulShutdown() {
	if atomic.SwapInt32(&ac.dbCleanupDone, 1) == 1 {
		// Already cleaned up
		return
	}

	fmt.Println("🔄 Thực hiện graceful shutdown...")

	// Save state including exporting pending emails
	ac.stateManager.SaveStateOnShutdown()
}

// Run starts the crawling process with SQLite integration
func (ac *AutoCrawler) Run() error {
	defer func() {
		// Ensure cleanup on exit
		ac.gracefulShutdown()

		if atomic.LoadInt32(&ac.shutdownRequested) == 0 {
			fmt.Printf("💤 Sleep %v trước khi thoát...\n", ac.config.SleepDuration)
			time.Sleep(ac.config.SleepDuration)
		}
	}()

	fmt.Printf("🚀 Bắt đầu Auto LinkedIn Crawler với SQLite\n")
	fmt.Printf("📊 Tổng số accounts: %d\n", len(ac.accounts))
	fmt.Printf("📧 Tổng số emails: %d\n", len(ac.totalEmails))
	fmt.Printf("🎯 Sẽ lấy %d tokens mỗi lần\n", ac.config.MaxTokens)

	// Show initial SQLite stats
	ac.stateManager.PrintDetailedStats()

	fmt.Println(strings.Repeat("=", 80))

	// Phase 1 - Xử lý tất cả emails
	if err := ac.batchProcessor.ProcessAllEmails(); err != nil {
		return err
	}

	// Phase 2 - Retry emails thất bại (only if not shutting down)
	if atomic.LoadInt32(&ac.shutdownRequested) == 0 {
		if err := ac.retryHandler.RetryFailedEmails(); err != nil {
			fmt.Printf("⚠️ Lỗi khi retry emails bị thất bại: %v\n", err)
		}
	}

	close(ac.logChan)
	ac.logWaitGroup.Wait()

	// Print final results
	ac.printFinalResults()

	return nil
}

// LogLine adds a line to the log channel
func (ac *AutoCrawler) LogLine(line string) {
	select {
	case ac.logChan <- line:
	default:
		fmt.Fprintf(os.Stderr, "⚠️ Log channel đầy, bỏ qua log: %s\n", line)
	}
}

// printFinalResults prints the final crawling results using SQLite stats
func (ac *AutoCrawler) printFinalResults() {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("🎉 HOÀN THÀNH AUTO LINKEDIN CRAWLER!")
	fmt.Println(strings.Repeat("=", 80))

	// Get final stats from SQLite (with error handling)
	stats, err := ac.stateManager.GetEmailStats()
	if err != nil {
		fmt.Printf("⚠️ Không thể lấy stats cuối cùng: %v\n", err)

		// Try to show alternative stats
		totalOriginal := len(ac.totalEmails)
		fmt.Printf("📈 TỔNG KẾT (LIMITED):\n")
		fmt.Printf("   📊 Tổng emails ban đầu:   %d\n", totalOriginal)
		fmt.Printf("   📁 Kết quả có thể xem trong file: %s\n", ac.outputFile)

		return
	}

	totalOriginal := len(ac.totalEmails)
	successCount := stats["success"]
	failedCount := stats["failed"]
	pendingCount := stats["pending"]
	hasInfoCount := stats["has_info"]
	noInfoCount := stats["no_info"]

	// Calculate percentages
	successPercent := 0.0
	if totalOriginal > 0 {
		successPercent = float64(successCount) * 100 / float64(totalOriginal)
	}

	dataPercent := 0.0
	if successCount > 0 {
		dataPercent = float64(hasInfoCount) * 100 / float64(successCount)
	}

	fmt.Printf("📈 TỔNG KẾT CUỐI CÙNG:\n")
	fmt.Printf("   📊 Tổng emails ban đầu:   %d\n", totalOriginal)
	fmt.Printf("   ✅ Đã xử lý thành công:  %d (%.1f%%)\n", successCount, successPercent)
	fmt.Printf("   ❌ Thất bại:             %d\n", failedCount)
	fmt.Printf("   ⏳ Chưa xử lý:           %d\n", pendingCount)
	fmt.Printf("   \n")
	fmt.Printf("   🎯 CÓ THÔNG TIN LINKEDIN: %d emails (%.1f%% trong thành công)\n", hasInfoCount, dataPercent)
	fmt.Printf("   📭 KHÔNG CÓ THÔNG TIN:   %d emails (%.1f%% trong thành công)\n", noInfoCount, 100-dataPercent)

	if hasInfoCount > 0 {
		fmt.Printf("\n🎉 TÌM THẤY %d PROFILES LINKEDIN - Kết quả trong file: %s\n", hasInfoCount, ac.outputFile)
	} else {
		fmt.Printf("\n😔 Không tìm thấy profile LinkedIn nào\n")
	}

	if pendingCount > 0 {
		fmt.Printf("\n💾 Còn %d emails chưa xử lý đã được lưu vào file %s\n", pendingCount, ac.config.EmailsFilePath)
	}

	fmt.Println(strings.Repeat("=", 80))
}

// PrintCurrentStats prints current processing statistics using SQLite
func (ac *AutoCrawler) PrintCurrentStats() {
	stats, err := ac.stateManager.GetEmailStats()
	if err != nil {
		fmt.Printf("⚠️ Không thể lấy stats: %v\n", err)
		return
	}

	total := len(ac.totalEmails)
	processed := stats["success"] + stats["failed"]

	fmt.Printf("📊 Stats: ✅%d 📭%d ❌%d ⏳%d | Progress: %d/%d (%.1f%%)\n",
		stats["has_info"], stats["no_info"], stats["failed"], stats["pending"],
		processed, total, float64(processed)*100/float64(total))
}

// Getter methods for service access
func (ac *AutoCrawler) GetConfig() models.Config {
	return ac.config
}

func (ac *AutoCrawler) GetTotalEmails() []string {
	return ac.totalEmails
}

func (ac *AutoCrawler) GetAccounts() []models.Account {
	return ac.accounts
}

func (ac *AutoCrawler) GetUsedAccountIndex() int {
	return ac.usedAccountIndex
}

func (ac *AutoCrawler) SetUsedAccountIndex(index int) {
	ac.usedAccountIndex = index
}

func (ac *AutoCrawler) GetOutputFile() string {
	return ac.outputFile
}

func (ac *AutoCrawler) GetStorageServices() (*storage.EmailStorage, *storage.TokenStorage, *storage.AccountStorage) {
	return ac.emailStorage, ac.tokenStorage, ac.accountStorage
}

// Legacy compatibility methods - now using SQLite
func (ac *AutoCrawler) GetEmailMaps() (map[string]struct{}, map[string]struct{}, map[string]struct{}, map[string]struct{}) {
	// Return empty maps since we're using SQLite now
	return make(map[string]struct{}), make(map[string]struct{}), make(map[string]struct{}), make(map[string]struct{})
}

func (ac *AutoCrawler) UpdateEmailMaps(withData, withoutData, failed, permanent map[string]struct{}) {
	// No-op since we're using SQLite now
}

func (ac *AutoCrawler) AddEmailToMap(email string, mapType string) {
	// Convert to SQLite operations
	switch mapType {
	case "withData":
		ac.emailStorage.UpdateEmailStatus(email, storage.StatusSuccess, true, false)
	case "withoutData":
		ac.emailStorage.UpdateEmailStatus(email, storage.StatusSuccess, false, true)
	case "failed":
		ac.emailStorage.UpdateEmailStatus(email, storage.StatusFailed, false, false)
	case "permanent":
		ac.emailStorage.UpdateEmailStatus(email, storage.StatusFailed, false, false)
	}
}

func (ac *AutoCrawler) GetShutdownRequested() *int32 {
	return &ac.shutdownRequested
}

func (ac *AutoCrawler) GetCrawler() *models.LinkedInCrawler {
	ac.crawlerMutex.RLock()
	defer ac.crawlerMutex.RUnlock()
	return ac.crawler
}

func (ac *AutoCrawler) SetCrawler(crawler *models.LinkedInCrawler) {
	ac.crawlerMutex.Lock()
	defer ac.crawlerMutex.Unlock()
	ac.crawler = crawler
}

func (ac *AutoCrawler) GetFileOpMutex() *sync.Mutex {
	return &ac.fileOpMutex
}
