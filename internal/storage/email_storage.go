package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// EmailStatus represents the status of an email
type EmailStatus string

const (
	StatusPending EmailStatus = "pending"
	StatusSuccess EmailStatus = "success"
	StatusFailed  EmailStatus = "failed"
)

// EmailRecord represents an email record in the database
type EmailRecord struct {
	ID      int         `json:"id"`
	Email   string      `json:"email"`
	Status  EmailStatus `json:"status"`
	HasInfo bool        `json:"has_info"`
	NoInfo  bool        `json:"no_info"`
}

// EmailStorage handles email file operations with SQLite backend
type EmailStorage struct {
	fileManager *FileManager
	db          *sql.DB
	dbPath      string
}

// NewEmailStorage creates a new EmailStorage instance
func NewEmailStorage() *EmailStorage {
	return &EmailStorage{
		fileManager: NewFileManager(),
		dbPath:      "emails.db",
	}
}

// InitDB initializes the SQLite database
func (es *EmailStorage) InitDB() error {
	var err error
	es.db, err = sql.Open("sqlite3", es.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := es.db.Ping(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Create table
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS emails (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL UNIQUE,
		status TEXT NOT NULL DEFAULT 'pending',
		has_info BOOLEAN DEFAULT FALSE,
		no_info BOOLEAN DEFAULT FALSE
	);
	CREATE INDEX IF NOT EXISTS idx_email_status ON emails(status);
	CREATE INDEX IF NOT EXISTS idx_email_email ON emails(email);
	`

	if _, err := es.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

// CloseDB closes the database connection
func (es *EmailStorage) CloseDB() error {
	if es.db != nil {
		return es.db.Close()
	}
	return nil
}

// isValidEmail validates email format
func (es *EmailStorage) isValidEmail(email string) bool {
	// Basic email regex pattern
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

// LoadEmailsFromFile loads emails from file, validates them, and imports to SQLite
func (es *EmailStorage) LoadEmailsFromFile(filePath string) ([]string, error) {
	// Initialize database
	if err := es.InitDB(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory for emails file: %w", err)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Printf("Emails file not found at %s, creating empty file\n", filePath)
		if err := os.WriteFile(filePath, []byte("example@example.com\n"), 0644); err != nil {
			return nil, fmt.Errorf("failed to create emails file: %w", err)
		}
	}

	lines, err := es.fileManager.ReadLines(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read emails file: %w", err)
	}

	// Drop existing table and recreate
	if _, err := es.db.Exec("DROP TABLE IF EXISTS emails"); err != nil {
		return nil, fmt.Errorf("failed to drop table: %w", err)
	}

	if err := es.InitDB(); err != nil {
		return nil, fmt.Errorf("failed to reinitialize database: %w", err)
	}

	var validEmails []string
	var invalidEmails []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		email := line

		if strings.Contains(line, ",") {
			parts := strings.SplitN(line, ",", 2)
			email = strings.TrimSpace(parts[1])
		}

		if email != "" && !strings.HasPrefix(email, "#") {
			if es.isValidEmail(email) {
				validEmails = append(validEmails, email)
			} else {
				invalidEmails = append(invalidEmails, email)
				fmt.Printf("⚠️ Email không hợp lệ, bỏ qua: %s\n", email)
			}
		}
	}

	if len(invalidEmails) > 0 {
		fmt.Printf("🗑️ Đã bỏ qua %d emails không hợp lệ\n", len(invalidEmails))
	}

	// Import valid emails to database
	if len(validEmails) > 0 {
		tx, err := es.db.Begin()
		if err != nil {
			return nil, fmt.Errorf("failed to begin transaction: %w", err)
		}

		stmt, err := tx.Prepare("INSERT OR IGNORE INTO emails (email, status) VALUES (?, ?)")
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer stmt.Close()

		for _, email := range validEmails {
			if _, err := stmt.Exec(email, StatusPending); err != nil {
				fmt.Printf("⚠️ Không thể thêm email %s: %v\n", email, err)
			}
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}

		fmt.Printf("✅ Đã import %d emails hợp lệ vào database\n", len(validEmails))
	}

	// Return pending emails
	return es.GetPendingEmails()
}

// GetPendingEmails returns all emails with pending status
func (es *EmailStorage) GetPendingEmails() ([]string, error) {
	if es.db == nil {
		if err := es.InitDB(); err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
	}

	rows, err := es.db.Query("SELECT email FROM emails WHERE status = ?", StatusPending)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending emails: %w", err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}
		emails = append(emails, email)
	}

	return emails, nil
}

// UpdateEmailStatus updates the status of an email
func (es *EmailStorage) UpdateEmailStatus(email string, status EmailStatus, hasInfo, noInfo bool) error {
	if es.db == nil {
		if err := es.InitDB(); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
	}

	_, err := es.db.Exec(
		"UPDATE emails SET status = ?, has_info = ?, no_info = ? WHERE email = ?",
		status, hasInfo, noInfo, email,
	)
	if err != nil {
		return fmt.Errorf("failed to update email status: %w", err)
	}

	return nil
}

// ExportPendingEmailsToFile exports pending emails back to file
func (es *EmailStorage) ExportPendingEmailsToFile(filePath string) error {
	if es.db == nil {
		if err := es.InitDB(); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
	}

	pendingEmails, err := es.GetPendingEmails()
	if err != nil {
		return fmt.Errorf("failed to get pending emails: %w", err)
	}

	return es.fileManager.WriteLines(filePath, pendingEmails)
}

// GetEmailStats returns statistics about emails
func (es *EmailStorage) GetEmailStats() (map[string]int, error) {
	if es.db == nil {
		if err := es.InitDB(); err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
	}

	stats := make(map[string]int)

	// Get counts by status
	rows, err := es.db.Query("SELECT status, COUNT(*) FROM emails GROUP BY status")
	if err != nil {
		return nil, fmt.Errorf("failed to get email stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan stats: %w", err)
		}
		stats[status] = count
	}

	// Initialize missing statuses
	if _, ok := stats["pending"]; !ok {
		stats["pending"] = 0
	}
	if _, ok := stats["success"]; !ok {
		stats["success"] = 0
	}
	if _, ok := stats["failed"]; !ok {
		stats["failed"] = 0
	}

	// Get has_info and no_info counts
	var hasInfoCount, noInfoCount int

	err = es.db.QueryRow("SELECT COUNT(*) FROM emails WHERE has_info = true").Scan(&hasInfoCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get has_info count: %w", err)
	}
	stats["has_info"] = hasInfoCount

	err = es.db.QueryRow("SELECT COUNT(*) FROM emails WHERE no_info = true").Scan(&noInfoCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get no_info count: %w", err)
	}
	stats["no_info"] = noInfoCount

	return stats, nil
}

// GetEmailsByStatus returns emails by status
func (es *EmailStorage) GetEmailsByStatus(status EmailStatus) ([]string, error) {
	if es.db == nil {
		if err := es.InitDB(); err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
	}

	rows, err := es.db.Query("SELECT email FROM emails WHERE status = ?", status)
	if err != nil {
		return nil, fmt.Errorf("failed to query emails by status: %w", err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}
		emails = append(emails, email)
	}

	return emails, nil
}

// WriteEmailsToFile writes emails to a file in a thread-safe manner (legacy compatibility)
func (es *EmailStorage) WriteEmailsToFile(filePath string, emails []string) error {
	return es.fileManager.WriteLines(filePath, emails)
}

// RemoveEmailFromFile removes an email from a file in a thread-safe manner (legacy compatibility)
func (es *EmailStorage) RemoveEmailFromFile(filePath string, emailToRemove string) error {
	return es.fileManager.RemoveLineFromFile(filePath, emailToRemove)
}
