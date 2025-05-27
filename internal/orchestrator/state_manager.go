package orchestrator

import (
	"fmt"
)

// StateManager handles state persistence and management with SQLite
type StateManager struct {
	autoCrawler *AutoCrawler
}

// NewStateManager creates a new StateManager instance
func NewStateManager(ac *AutoCrawler) *StateManager {
	return &StateManager{
		autoCrawler: ac,
	}
}

// HasEmailsToProcess checks if there are emails left to process (pending status)
func (sm *StateManager) HasEmailsToProcess() bool {
	emailStorage, _, _ := sm.autoCrawler.GetStorageServices()

	pendingEmails, err := emailStorage.GetPendingEmails()
	if err != nil {
		fmt.Printf("⚠️ Không thể kiểm tra pending emails: %v\n", err)
		return false
	}

	return len(pendingEmails) > 0
}

// CountRemainingEmails counts how many emails are left to process (pending status)
func (sm *StateManager) CountRemainingEmails() int {
	emailStorage, _, _ := sm.autoCrawler.GetStorageServices()

	pendingEmails, err := emailStorage.GetPendingEmails()
	if err != nil {
		fmt.Printf("⚠️ Không thể đếm pending emails: %v\n", err)
		return 0
	}

	return len(pendingEmails)
}

// GetRemainingEmails returns the list of emails that still need processing (pending status)
func (sm *StateManager) GetRemainingEmails() []string {
	emailStorage, _, _ := sm.autoCrawler.GetStorageServices()

	pendingEmails, err := emailStorage.GetPendingEmails()
	if err != nil {
		fmt.Printf("⚠️ Không thể lấy pending emails: %v\n", err)
		return []string{}
	}

	return pendingEmails
}

// SaveStateOnShutdown saves the current state when shutting down - exports pending emails to file
func (sm *StateManager) SaveStateOnShutdown() {
	emailStorage, _, _ := sm.autoCrawler.GetStorageServices()
	config := sm.autoCrawler.GetConfig()

	// Get current stats
	stats, err := emailStorage.GetEmailStats()
	if err != nil {
		fmt.Printf("⚠️ Không thể lấy stats khi shutdown: %v\n", err)
		return
	}

	// Export pending emails back to file
	err = emailStorage.ExportPendingEmailsToFile(config.EmailsFilePath)
	if err != nil {
		fmt.Printf("⚠️ Không thể export pending emails khi shutdown: %v\n", err)
		return
	}

	pendingCount := stats["pending"]
	successCount := stats["success"]
	failedCount := stats["failed"]

	if pendingCount == 0 {
		fmt.Println("📝 Tất cả emails đã được xử lý - file emails.txt trống")
	} else {
		fmt.Printf("💾 Đã lưu %d emails pending vào file emails.txt\n", pendingCount)
	}

	fmt.Printf("📊 Tổng kết: Success: %d | Failed: %d | Pending: %d | HasInfo: %d | NoInfo: %d\n",
		successCount, failedCount, pendingCount, stats["has_info"], stats["no_info"])

	// Close database connection
	if err := emailStorage.CloseDB(); err != nil {
		fmt.Printf("⚠️ Lỗi khi đóng database: %v\n", err)
	}
}

// UpdateEmailsFile updates the emails file with pending emails (legacy compatibility)
func (sm *StateManager) UpdateEmailsFile() {
	emailStorage, _, _ := sm.autoCrawler.GetStorageServices()
	config := sm.autoCrawler.GetConfig()

	err := emailStorage.ExportPendingEmailsToFile(config.EmailsFilePath)
	if err != nil {
		fmt.Printf("⚠️ Không thể cập nhật emails file: %v\n", err)
		return
	}

	pendingEmails, err := emailStorage.GetPendingEmails()
	if err != nil {
		fmt.Printf("⚠️ Không thể lấy pending emails: %v\n", err)
		return
	}

	fmt.Printf("💾 Đã cập nhật file emails: %d emails pending còn lại\n", len(pendingEmails))
}

// GetEmailStats returns current email statistics from SQLite
func (sm *StateManager) GetEmailStats() (map[string]int, error) {
	emailStorage, _, _ := sm.autoCrawler.GetStorageServices()
	return emailStorage.GetEmailStats()
}

// PrintDetailedStats prints detailed statistics from SQLite
func (sm *StateManager) PrintDetailedStats() {
	stats, err := sm.GetEmailStats()
	if err != nil {
		fmt.Printf("⚠️ Không thể lấy stats: %v\n", err)
		return
	}

	fmt.Printf("📊 Chi tiết thống kê từ SQLite:\n")
	fmt.Printf("   ✅ Success: %d emails\n", stats["success"])
	fmt.Printf("   ❌ Failed: %d emails\n", stats["failed"])
	fmt.Printf("   ⏳ Pending: %d emails\n", stats["pending"])
	fmt.Printf("   🎯 Có thông tin LinkedIn: %d emails\n", stats["has_info"])
	fmt.Printf("   📭 Không có thông tin: %d emails\n", stats["no_info"])

	total := stats["success"] + stats["failed"] + stats["pending"]
	if total > 0 {
		successPercent := float64(stats["success"]) * 100 / float64(total)
		fmt.Printf("   📈 Tỷ lệ thành công: %.1f%%\n", successPercent)

		if stats["success"] > 0 {
			dataPercent := float64(stats["has_info"]) * 100 / float64(stats["success"])
			fmt.Printf("   🎯 Tỷ lệ có data trong thành công: %.1f%%\n", dataPercent)
		}
	}
}
