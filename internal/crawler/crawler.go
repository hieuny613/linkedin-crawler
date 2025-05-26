package crawler

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sync/semaphore"

	"linkedin-crawler/internal/models"
)

// New creates a new LinkedInCrawler instance
func New(config models.Config, outputFilePath string) (*models.LinkedInCrawler, error) {
	transport := &http.Transport{
		MaxIdleConns:           int(config.MaxConcurrency),
		MaxIdleConnsPerHost:    int(config.MaxConcurrency),
		MaxConnsPerHost:        int(config.MaxConcurrency),
		IdleConnTimeout:        30 * time.Second,
		DisableCompression:     false,
		ForceAttemptHTTP2:      true,
		DisableKeepAlives:      false,
		MaxResponseHeaderBytes: 1 << 20, // 1MB limit
		ResponseHeaderTimeout:  10 * time.Second,
		ExpectContinueTimeout:  1 * time.Second,
	}

	client := &http.Client{
		Timeout:   config.RequestTimeout,
		Transport: transport,
	}

	if err := os.MkdirAll(filepath.Dir(outputFilePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// APPEND mode - ghi thêm vào file hit.txt (KHÔNG ghi đè)
	outputFile, err := os.OpenFile(outputFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}

	bufferedWriter := bufio.NewWriter(outputFile)

	// Tạo context để cleanup goroutines
	ctx, cancel := context.WithCancel(context.Background())

	// Giảm buffer size để tiết kiệm memory
	requestChan := make(chan struct{}, 50)

	// Fill initial tokens với context
	for i := 0; i < 50 && i < int(config.RequestsPerSec); i++ {
		select {
		case requestChan <- struct{}{}:
		case <-ctx.Done():
			break
		}
	}

	// Start ticker goroutine với context cleanup
	requestTicker := time.NewTicker(time.Second / time.Duration(config.RequestsPerSec))
	go func() {
		defer requestTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return // Cleanup goroutine khi context cancel
			case <-requestTicker.C:
				select {
				case requestChan <- struct{}{}:
				default:
					// Channel is full, skip this tick
				}
			}
		}
	}()

	return &models.LinkedInCrawler{
		Client:            client,
		MaxConcurrency:    config.MaxConcurrency,
		Sem:               semaphore.NewWeighted(config.MaxConcurrency),
		OutputFile:        outputFile,
		BufferedWriter:    bufferedWriter,
		StartTime:         time.Now(),
		InvalidTokens:     make(map[string]bool),
		TokensFilePath:    config.TokensFilePath,
		RateLimitedEmails: []string{},
		RequestSemaphore:  semaphore.NewWeighted(config.MaxConcurrency),
		RequestTicker:     requestTicker,
		RequestChan:       requestChan,
		Ctx:               ctx,
		Cancel:            cancel,
	}, nil
}

// Close cleans up resources to prevent memory leaks
func Close(lc *models.LinkedInCrawler) error {
	if lc.Cancel != nil {
		lc.Cancel()
	}

	if lc.RequestTicker != nil {
		lc.RequestTicker.Stop()
	}

	if lc.RequestChan != nil {
		// Drain channel
		for {
			select {
			case <-lc.RequestChan:
			default:
				close(lc.RequestChan)
				goto channelClosed
			}
		}
	channelClosed:
		lc.RequestChan = nil
	}

	// Close HTTP transport connections
	if lc.Client != nil && lc.Client.Transport != nil {
		if transport, ok := lc.Client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}

	if lc.BufferedWriter != nil {
		if err := lc.BufferedWriter.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "Error flushing buffer: %v", err)
		}
		lc.BufferedWriter = nil
	}

	if lc.OutputFile != nil {
		err := lc.OutputFile.Close()
		lc.OutputFile = nil
		return err
	}

	return nil
}
