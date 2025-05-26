package models

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
)

// LinkedInCrawler represents the core LinkedIn crawler
type LinkedInCrawler struct {
	Tokens         []string
	InvalidTokens  map[string]bool
	CurrentToken   int32
	Client         *http.Client
	MaxConcurrency int64
	Sem            *semaphore.Weighted
	RateLimiter    <-chan time.Time
	OutputFile     *os.File
	BufferedWriter *bufio.Writer
	OutputMutex    sync.Mutex
	Stats          struct {
		Processed   int32
		Success     int32
		Failed      int32
		TokenErrors int32
	}
	StartTime         time.Time
	AllTokensFailed   bool
	TokenMutex        sync.Mutex
	TokensFilePath    string
	RateLimitedEmails []string
	RateLimitMutex    sync.Mutex
	ActiveRequests    int32
	RequestSemaphore  *semaphore.Weighted
	RequestTicker     *time.Ticker
	RequestChan       chan struct{}
	Ctx               context.Context
	Cancel            context.CancelFunc
}

// AutoCrawler represents the main orchestrator for the LinkedIn crawler
type AutoCrawler struct {
	Config            Config
	Accounts          []Account
	UsedAccountIndex  int
	Crawler           *LinkedInCrawler
	CrawlerMutex      sync.RWMutex
	OutputFile        string
	TotalEmails       []string
	ProcessedEmails   int
	ShutdownRequested int32

	LogFile      *os.File
	LogWriter    *bufio.Writer
	LogChan      chan string
	LogWaitGroup sync.WaitGroup

	// Email tracking maps
	SuccessEmailsWithData    map[string]struct{} // Emails có thông tin LinkedIn
	SuccessEmailsWithoutData map[string]struct{} // Emails không có thông tin LinkedIn
	FailedEmails             map[string]struct{} // Emails thất bại cần retry
	PermanentFailed          map[string]struct{} // Emails lỗi vĩnh viễn
	EmailsMutex              sync.Mutex

	// File operation mutex để tránh race condition
	FileOpMutex sync.Mutex
}

// LinkedInCrawler Methods

// GetToken returns a random valid token
func (lc *LinkedInCrawler) GetToken() string {
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

	// Simple random selection
	idx := int(time.Now().UnixNano()) % len(validTokens)
	return validTokens[idx]
}

// AreAllTokensFailed checks if all tokens have failed
func (lc *LinkedInCrawler) AreAllTokensFailed() bool {
	lc.TokenMutex.Lock()
	defer lc.TokenMutex.Unlock()
	return lc.AllTokensFailed
}

// WriteToFile writes profile data to output file
func (lc *LinkedInCrawler) WriteToFile(email string, profile ProfileData) error {
	lc.OutputMutex.Lock()
	defer lc.OutputMutex.Unlock()

	line := fmt.Sprintf("%s|%s|%s|%s|%s\n", email, profile.User, profile.LinkedInURL, profile.Location, profile.ConnectionCount)
	_, err := lc.BufferedWriter.WriteString(line)
	if err != nil {
		return fmt.Errorf("failed to write to output file: %w", err)
	}

	// Force flush
	if flushErr := lc.BufferedWriter.Flush(); flushErr != nil {
		return fmt.Errorf("failed to flush output file: %w", flushErr)
	}

	// Force sync to disk
	if syncErr := lc.OutputFile.Sync(); syncErr != nil {
		return fmt.Errorf("failed to sync output file: %w", syncErr)
	}

	return nil
}
