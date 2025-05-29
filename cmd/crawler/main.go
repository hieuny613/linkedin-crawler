package main

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"linkedin-crawler/internal/config"
	"linkedin-crawler/internal/orchestrator"
	"linkedin-crawler/internal/storage"
	"linkedin-crawler/internal/utils"
)

func main() {
	fmt.Println("🚀 LinkedIn Auto Crawler - Refactored Version")
	fmt.Println(strings.Repeat("=", 60))

	// Load configuration
	cfg := config.DefaultConfig()

	// Create auto crawler
	autoCrawler, err := orchestrator.New(cfg)
	if err != nil {
		log.Fatalf("❌ Lỗi khởi tạo auto crawler: %v", err)
	}
	emailStorage, _, _ := autoCrawler.GetStorageServices()
	if err := dropEmailsTable(emailStorage); err != nil {
		log.Fatalf("❌ %v", err)
	}
	// Start crawling
	startTime := time.Now()
	err = autoCrawler.Run()
	duration := time.Since(startTime)

	if err != nil {
		log.Printf("❌ Lỗi trong quá trình chạy: %v", err)
	}

	fmt.Printf("🎉 Hoàn thành trong %s\n", utils.FormatDuration(duration))
	fmt.Printf("📊 Kết quả được lưu trong file: %s\n", autoCrawler.GetOutputFile())

	// Memory stats để kiểm tra memory leaks
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("💾 Memory: Alloc=%d KB, TotalAlloc=%d KB, Sys=%d KB, NumGC=%d\n",
		m.Alloc/1024, m.TotalAlloc/1024, m.Sys/1024, m.NumGC)

	fmt.Println(strings.Repeat("=", 60))
}

func dropEmailsTable(es *storage.EmailStorage) error {
	// Execute DROP TABLE IF EXISTS
	if _, err := es.GetDB().Exec("DROP TABLE IF EXISTS emails"); err != nil {
		return fmt.Errorf("failed to drop existing emails table: %w", err)
	}
	fmt.Println("✅ Dropped existing emails table")
	return nil
}
