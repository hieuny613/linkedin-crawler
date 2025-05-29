package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

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
	dbMutex     sync.RWMutex // Protect database access
	isDBClosed  bool         // Track if DB is closed
}

// NewEmailStorage creates a new EmailStorage instance
func NewEmailStorage() *EmailStorage {
	return &EmailStorage{
		fileManager: NewFileManager(),
		dbPath:      "emails.db",
		isDBClosed:  false,
	}
}

// InitDB initializes the SQLite database and DROPS existing table
func (es *EmailStorage) InitDB() error {
	es.dbMutex.Lock()
	defer es.dbMutex.Unlock()

	// If already initialized and not closed, return
	if es.db != nil && !es.isDBClosed {
		return nil
	}

	var err error
	es.db, err = sql.Open("sqlite3", es.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := es.db.Ping(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	es.isDBClosed = false

	// // IMPORTANT: Drop existing table first to start fresh
	// if _, err := es.db.Exec("DROP TABLE IF EXISTS emails"); err != nil {
	// 	return fmt.Errorf("failed to drop existing emails table: %w", err)
	// }

	// Create fresh table
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS emails (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL UNIQUE,
		status TEXT NOT NULL DEFAULT 'pending',
		has_info BOOLEAN DEFAULT FALSE,
		no_info BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_email_status ON emails(status);
	CREATE INDEX IF NOT EXISTS idx_email_email ON emails(email);
	CREATE INDEX IF NOT EXISTS idx_email_has_info ON emails(has_info);
	CREATE INDEX IF NOT EXISTS idx_email_no_info ON emails(no_info);
	`

	if _, err := es.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create emails table: %w", err)
	}
	return nil
}

// CloseDB closes the database connection
func (es *EmailStorage) CloseDB() error {
	es.dbMutex.Lock()
	defer es.dbMutex.Unlock()

	if es.db != nil && !es.isDBClosed {
		es.isDBClosed = true
		return es.db.Close()
	}
	return nil
}

// ensureDB ensures database connection is available
func (es *EmailStorage) ensureDB() error {
	es.dbMutex.RLock()
	dbOpen := es.db != nil && !es.isDBClosed
	es.dbMutex.RUnlock()

	if dbOpen {
		// DB đang mở bình thường → tiếp tục
		return nil
	}
	if es.db != nil && es.isDBClosed {
		// DB đã từng mở rồi nhưng đã đóng → không tái khởi tạo (tránh mất data)
		return fmt.Errorf("database has been closed")
	}
	// Trường hợp lần đầu khởi tạo
	return es.InitDB()
}

// isValidEmail validates email format
func (es *EmailStorage) isValidEmail(email string) bool {
	// Basic email regex pattern
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

// LoadEmailsFromFile loads emails from file, validates them, and imports to SQLite
// ALWAYS drops and recreates table for fresh start
func (es *EmailStorage) LoadEmailsFromFile(filePath string) ([]string, error) {
	// Initialize database (this will drop existing table)
	if err := es.ensureDB(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	if err := es.ensureDB(); err != nil {
		return nil, fmt.Errorf("failed to initialize database before drop: %w", err)
	}
	if _, err := es.db.Exec("DROP TABLE IF EXISTS emails"); err != nil {
		return nil, fmt.Errorf("failed to drop existing emails table: %w", err)
	}
	// Sau khi drop, cần tạo lại schema (tương tự InitDB)
	if _, err := es.db.Exec(`
        CREATE TABLE IF NOT EXISTS emails (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            email TEXT UNIQUE,
            status TEXT,
            has_info BOOLEAN,
            no_info BOOLEAN,
            updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )
    `); err != nil {
		return nil, fmt.Errorf("failed to recreate emails table: %w", err)
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory for emails file: %w", err)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Printf("Emails file not found at %s, creating sample file\n", filePath)
		sampleContent := `# Target email addresses
# One email per line
example@example.com
test@test.com
`
		if err := os.WriteFile(filePath, []byte(sampleContent), 0644); err != nil {
			return nil, fmt.Errorf("failed to create emails file: %w", err)
		}
	}

	lines, err := es.fileManager.ReadLines(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read emails file: %w", err)
	}

	es.dbMutex.Lock()
	defer es.dbMutex.Unlock()

	if es.isDBClosed {
		return nil, fmt.Errorf("database is closed")
	}

	// Parse and validate emails
	var validEmails []string
	var invalidEmails []string

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		email := line

		// Handle CSV format (take first valid email from comma-separated values)
		if strings.Contains(line, ",") {
			parts := strings.SplitN(line, ",", 2)
			email = strings.TrimSpace(parts[0])
		}

		if email != "" {
			if es.isValidEmail(email) {
				validEmails = append(validEmails, email)
			} else {
				invalidEmails = append(invalidEmails, email)
				fmt.Printf("⚠️ Line %d - Invalid email format, skipped: %s\n", lineNum+1, email)
			}
		}
	}

	if len(invalidEmails) > 0 {
		fmt.Printf("🗑️ Skipped %d invalid emails\n", len(invalidEmails))
	}

	// Remove duplicates
	emailMap := make(map[string]bool)
	uniqueEmails := []string{}
	duplicates := 0

	for _, email := range validEmails {
		email = strings.ToLower(email) // Normalize to lowercase
		if !emailMap[email] {
			emailMap[email] = true
			uniqueEmails = append(uniqueEmails, email)
		} else {
			duplicates++
		}
	}

	if duplicates > 0 {
		fmt.Printf("🔄 Removed %d duplicate emails\n", duplicates)
	}

	// Import unique valid emails to database
	if len(uniqueEmails) > 0 {
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

		inserted := 0
		for _, email := range uniqueEmails {
			result, err := stmt.Exec(email, StatusPending)
			if err != nil {
				fmt.Printf("⚠️ Failed to insert email %s: %v\n", email, err)
				continue
			}

			// Check if actually inserted (not ignored due to duplicate)
			if rowsAffected, _ := result.RowsAffected(); rowsAffected > 0 {
				inserted++
			}
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}

		fmt.Printf("✅ Imported %d unique emails to database\n", inserted)
	}

	// Return all pending emails from database
	rows, err := es.db.Query("SELECT email FROM emails WHERE status = ? ORDER BY id", StatusPending)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending emails: %w", err)
	}
	defer rows.Close()

	var pendingEmails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}
		pendingEmails = append(pendingEmails, email)
	}

	fmt.Printf("📊 Database summary: %d pending emails ready for processing\n", len(pendingEmails))
	return pendingEmails, nil
}

// GetPendingEmails returns all emails with pending status
func (es *EmailStorage) GetPendingEmails() ([]string, error) {
	if err := es.ensureDB(); err != nil {
		return nil, fmt.Errorf("failed to ensure database: %w", err)
	}

	es.dbMutex.RLock()
	defer es.dbMutex.RUnlock()

	if es.isDBClosed {
		return nil, fmt.Errorf("database is closed")
	}

	rows, err := es.db.Query("SELECT email FROM emails WHERE status = ? ORDER BY id", StatusPending)
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
	if err := es.ensureDB(); err != nil {
		return fmt.Errorf("failed to ensure database: %w", err)
	}

	es.dbMutex.RLock()
	defer es.dbMutex.RUnlock()

	if es.isDBClosed {
		return fmt.Errorf("database is closed")
	}

	_, err := es.db.Exec(
		"UPDATE emails SET status = ?, has_info = ?, no_info = ?, updated_at = CURRENT_TIMESTAMP WHERE email = ?",
		status, hasInfo, noInfo, email,
	)
	if err != nil {
		return fmt.Errorf("failed to update email status: %w", err)
	}

	return nil
}

// ExportPendingEmailsToFile exports pending emails back to file
func (es *EmailStorage) ExportPendingEmailsToFile(filePath string) error {
	pendingEmails, err := es.GetPendingEmails()
	if err != nil {
		return fmt.Errorf("failed to get pending emails: %w", err)
	}

	// Add header comment
	var lines []string
	lines = append(lines, "# Pending emails for LinkedIn crawler")
	lines = append(lines, fmt.Sprintf("# Exported on: %s", strings.Split(fmt.Sprintf("%v", time.Now()), " ")[0]))
	lines = append(lines, fmt.Sprintf("# Total pending: %d", len(pendingEmails)))
	lines = append(lines, "")

	// Add emails
	lines = append(lines, pendingEmails...)

	return es.fileManager.WriteLines(filePath, lines)
}

// GetEmailStats returns statistics about emails
func (es *EmailStorage) GetEmailStats() (map[string]int, error) {
	if err := es.ensureDB(); err != nil {
		return nil, fmt.Errorf("failed to ensure database: %w", err)
	}

	es.dbMutex.RLock()
	defer es.dbMutex.RUnlock()

	if es.isDBClosed {
		return nil, fmt.Errorf("database is closed")
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
	if err := es.ensureDB(); err != nil {
		return nil, fmt.Errorf("failed to ensure database: %w", err)
	}

	es.dbMutex.RLock()
	defer es.dbMutex.RUnlock()

	if es.isDBClosed {
		return nil, fmt.Errorf("database is closed")
	}

	rows, err := es.db.Query("SELECT email FROM emails WHERE status = ? ORDER BY id", status)
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

// GetDatabaseInfo returns information about the database
func (es *EmailStorage) GetDatabaseInfo() (map[string]interface{}, error) {
	if err := es.ensureDB(); err != nil {
		return nil, fmt.Errorf("failed to ensure database: %w", err)
	}

	es.dbMutex.RLock()
	defer es.dbMutex.RUnlock()

	if es.isDBClosed {
		return nil, fmt.Errorf("database is closed")
	}

	info := make(map[string]interface{})

	// Get total count
	var totalCount int
	err := es.db.QueryRow("SELECT COUNT(*) FROM emails").Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get total count: %w", err)
	}
	info["total_emails"] = totalCount

	// Get database file size
	if stat, err := os.Stat(es.dbPath); err == nil {
		info["db_file_size"] = stat.Size()
	}

	info["db_path"] = es.dbPath
	info["is_closed"] = es.isDBClosed

	return info, nil
}

// ResetDatabase drops and recreates the emails table (for testing/reset purposes)
func (es *EmailStorage) ResetDatabase() error {
	es.dbMutex.Lock()
	defer es.dbMutex.Unlock()

	if es.db == nil || es.isDBClosed {
		return fmt.Errorf("database is not initialized or closed")
	}

	// Drop existing table
	if _, err := es.db.Exec("DROP TABLE IF EXISTS emails"); err != nil {
		return fmt.Errorf("failed to drop emails table: %w", err)
	}

	// Create fresh table
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS emails (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL UNIQUE,
		status TEXT NOT NULL DEFAULT 'pending',
		has_info BOOLEAN DEFAULT FALSE,
		no_info BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_email_status ON emails(status);
	CREATE INDEX IF NOT EXISTS idx_email_email ON emails(email);
	CREATE INDEX IF NOT EXISTS idx_email_has_info ON emails(has_info);
	CREATE INDEX IF NOT EXISTS idx_email_no_info ON emails(no_info);
	`

	if _, err := es.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to recreate emails table: %w", err)
	}

	fmt.Println("✅ Database reset: Emails table dropped and recreated")
	return nil
}

// WriteEmailsToFile writes emails to a file in a thread-safe manner (legacy compatibility)
func (es *EmailStorage) WriteEmailsToFile(filePath string, emails []string) error {
	return es.fileManager.WriteLines(filePath, emails)
}

// RemoveEmailFromFile removes an email from a file in a thread-safe manner (legacy compatibility)
func (es *EmailStorage) RemoveEmailFromFile(filePath string, emailToRemove string) error {
	return es.fileManager.RemoveLineFromFile(filePath, emailToRemove)
}
func (es *EmailStorage) GetDB() *sql.DB {
	es.dbMutex.RLock()
	defer es.dbMutex.RUnlock()

	if es.isDBClosed {
		return nil
	}

	return es.db
}
