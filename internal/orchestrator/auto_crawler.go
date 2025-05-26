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

// AutoCrawler orchestrates the LinkedIn crawling process
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

	// Email tracking maps
	successEmailsWithData    map[string]struct{} // Emails có thông tin LinkedIn
	successEmailsWithoutData map[string]struct{} // Emails không có thông tin LinkedIn
	failedEmails             map[string]struct{} // Emails thất bại cần retry
	permanentFailed          map[string]struct{} // Emails lỗi vĩnh viễn
	emailsMutex              sync.Mutex

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
}

// New creates a new AutoCrawler instance
func New(config models.Config) (*AutoCrawler, error) {
	outputFile := "hit.txt"

	// Initialize storage services
	emailStorage := storage.NewEmailStorage()
	tokenStorage := storage.NewTokenStorage()
	accountStorage := storage.NewAccountStorage()

	// Load accounts and emails
	accounts, err := accountStorage.LoadAccounts(config.AccountsFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load accounts: %w", err)
	}

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

		// Initialize email tracking maps
		successEmailsWithData:    make(map[string]struct{}),
		successEmailsWithoutData: make(map[string]struct{}),
		failedEmails:             make(map[string]struct{}),
		permanentFailed:          make(map[string]struct{}),

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
	utils.SetupSignalHandling(&ac.shutdownRequested, ac.stateManager.SaveStateOnShutdown, config.SleepDuration)

	return ac, nil
}

// Run starts the crawling process
func (ac *AutoCrawler) Run() error {
	defer func() {
		if atomic.LoadInt32(&ac.shutdownRequested) == 0 {
			fmt.Printf("💤 Sleep %v trước khi thoát...\n", ac.config.SleepDuration)
			time.Sleep(ac.config.SleepDuration)
		}
	}()

	fmt.Printf("🚀 Bắt đầu Auto LinkedIn Crawler\n")
	fmt.Printf("📊 Tổng số accounts: %d\n", len(ac.accounts))
	fmt.Printf("📧 Tổng số emails: %d\n", len(ac.totalEmails))
	fmt.Printf("🎯 Sẽ lấy %d tokens mỗi lần\n", ac.config.MaxTokens)
	fmt.Println(strings.Repeat("=", 80))

	// Phase 1 - Xử lý tất cả emails
	if err := ac.batchProcessor.ProcessAllEmails(); err != nil {
		return err
	}

	// Phase 2 - Retry emails thất bại
	if err := ac.retryHandler.RetryFailedEmails(); err != nil {
		fmt.Printf("⚠️ Lỗi khi retry emails bị thất bại: %v\n", err)
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

// printFinalResults prints the final crawling results
func (ac *AutoCrawler) printFinalResults() {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("🎉 HOÀN THÀNH AUTO LINKEDIN CRAWLER!")
	fmt.Println(strings.Repeat("=", 80))

	ac.emailsMutex.Lock()
	withDataCount := len(ac.successEmailsWithData)
	withoutDataCount := len(ac.successEmailsWithoutData)
	failedCount := len(ac.failedEmails)
	permanentFailedCount := len(ac.permanentFailed)
	totalProcessed := withDataCount + withoutDataCount + permanentFailedCount
	totalOriginal := len(ac.totalEmails)
	ac.emailsMutex.Unlock()

	// Calculate percentages
	successPercent := float64(withDataCount+withoutDataCount) * 100 / float64(totalOriginal)
	dataPercent := 0.0
	if withDataCount+withoutDataCount > 0 {
		dataPercent = float64(withDataCount) * 100 / float64(withDataCount+withoutDataCount)
	}

	fmt.Printf("📈 TỔNG KẾT CUỐI CÙNG:\n")
	fmt.Printf("   📊 Tổng emails xử lý:     %d/%d (%.1f%%)\n", totalProcessed, totalOriginal, float64(totalProcessed)*100/float64(totalOriginal))
	fmt.Printf("   ✅ Thành công:           %d/%d (%.1f%%)\n", withDataCount+withoutDataCount, totalOriginal, successPercent)
	fmt.Printf("   \n")
	fmt.Printf("   🎯 CÓ THÔNG TIN LINKEDIN: %d emails (%.1f%% trong thành công)\n", withDataCount, dataPercent)
	fmt.Printf("   📭 KHÔNG CÓ THÔNG TIN:   %d emails (%.1f%% trong thành công)\n", withoutDataCount, 100-dataPercent)
	fmt.Printf("   \n")
	fmt.Printf("   ❌ Cần retry:            %d emails\n", failedCount)
	fmt.Printf("   💀 Lỗi vĩnh viễn:        %d emails\n", permanentFailedCount)

	if withDataCount > 0 {
		fmt.Printf("\n🎉 TÌM THẤY %d PROFILES LINKEDIN - Kết quả trong file: %s\n", withDataCount, ac.outputFile)
	} else {
		fmt.Printf("\n😔 Không tìm thấy profile LinkedIn nào\n")
	}

	fmt.Println(strings.Repeat("=", 80))
}

// PrintCurrentStats prints current processing statistics
func (ac *AutoCrawler) PrintCurrentStats() {
	ac.emailsMutex.Lock()
	withData := len(ac.successEmailsWithData)
	withoutData := len(ac.successEmailsWithoutData)
	failed := len(ac.failedEmails)
	permanent := len(ac.permanentFailed)
	total := len(ac.totalEmails)
	ac.emailsMutex.Unlock()

	processed := withData + withoutData + permanent
	fmt.Printf("📊 Stats: ✅%d 📭%d ❌%d 💀%d | Progress: %d/%d (%.1f%%)\n",
		withData, withoutData, failed, permanent, processed, total, float64(processed)*100/float64(total))
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

func (ac *AutoCrawler) GetEmailMaps() (map[string]struct{}, map[string]struct{}, map[string]struct{}, map[string]struct{}) {
	ac.emailsMutex.Lock()
	defer ac.emailsMutex.Unlock()

	// Return copies to prevent external modification
	withData := make(map[string]struct{})
	withoutData := make(map[string]struct{})
	failed := make(map[string]struct{})
	permanent := make(map[string]struct{})

	for k, v := range ac.successEmailsWithData {
		withData[k] = v
	}
	for k, v := range ac.successEmailsWithoutData {
		withoutData[k] = v
	}
	for k, v := range ac.failedEmails {
		failed[k] = v
	}
	for k, v := range ac.permanentFailed {
		permanent[k] = v
	}

	return withData, withoutData, failed, permanent
}

func (ac *AutoCrawler) UpdateEmailMaps(withData, withoutData, failed, permanent map[string]struct{}) {
	ac.emailsMutex.Lock()
	defer ac.emailsMutex.Unlock()

	ac.successEmailsWithData = withData
	ac.successEmailsWithoutData = withoutData
	ac.failedEmails = failed
	ac.permanentFailed = permanent
}

func (ac *AutoCrawler) AddEmailToMap(email string, mapType string) {
	ac.emailsMutex.Lock()
	defer ac.emailsMutex.Unlock()

	switch mapType {
	case "withData":
		ac.successEmailsWithData[email] = struct{}{}
	case "withoutData":
		ac.successEmailsWithoutData[email] = struct{}{}
	case "failed":
		ac.failedEmails[email] = struct{}{}
	case "permanent":
		ac.permanentFailed[email] = struct{}{}
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
