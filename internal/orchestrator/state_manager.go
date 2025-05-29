package orchestrator

import (
	"fmt"
	"linkedin-crawler/internal/storage"
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
		// If database error, assume no emails to be safe
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
	config := sm.autoCrawler.GetConfig()
	fmt.Println("💾 Đang lưu trạng thái trước khi thoát…")

	// 1) Mở 1 kết nối DB mới riêng cho việc export
	freshStorage := storage.NewEmailStorage()
	if err := freshStorage.InitDB(); err != nil {
		fmt.Printf("⚠️ Không thể mở DB để export: %v\n", err)
		return
	}
	defer func() {
		if err := freshStorage.CloseDB(); err != nil {
			fmt.Printf("⚠️ Lỗi khi đóng DB: %v\n", err)
		} else {
			fmt.Println("✅ Đã đóng DB connection (shutdown)")
		}
	}()

	// 2) Export pending emails về file
	if err := freshStorage.ExportPendingEmailsToFile(config.EmailsFilePath); err != nil {
		fmt.Printf("⚠️ Không thể export pending emails: %v\n", err)
	} else {
		fmt.Println("💾 Đã export pending emails thành công")
	}

	// 3) Lấy stats cuối cùng
	stats, err := freshStorage.GetEmailStats()
	if err != nil {
		fmt.Printf("⚠️ Không thể lấy stats cuối: %v\n", err)
	} else {
		fmt.Printf(
			"📊 Tổng kết: Success: %d | Failed: %d | Pending: %d | HasInfo: %d | NoInfo: %d\n",
			stats["success"], stats["failed"], stats["pending"],
			stats["has_info"], stats["no_info"],
		)
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

// GetEmailStats returns current email statistics from SQLite with error handling
func (sm *StateManager) GetEmailStats() (map[string]int, error) {
	emailStorage, _, _ := sm.autoCrawler.GetStorageServices()

	stats, err := emailStorage.GetEmailStats()
	if err != nil {
		// Return empty stats instead of nil to avoid panics
		return map[string]int{
			"pending":  0,
			"success":  0,
			"failed":   0,
			"has_info": 0,
			"no_info":  0,
		}, err
	}

	return stats, nil
}

// PrintDetailedStats prints detailed statistics from SQLite with error handling
func (sm *StateManager) PrintDetailedStats() {
	stats, err := sm.GetEmailStats()
	if err != nil {
		fmt.Printf("⚠️ Không thể lấy stats: %v\n", err)
		// Show fallback info
		fmt.Printf("📊 Chi tiết thống kê: Không khả dụng (database error)\n")
		fmt.Printf("   📧 Tổng emails từ file: %d\n", len(sm.autoCrawler.GetTotalEmails()))
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
