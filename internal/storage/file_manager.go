package storage

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileManager handles thread-safe file operations
type FileManager struct {
	mutex sync.Mutex
}

// NewFileManager creates a new FileManager instance
func NewFileManager() *FileManager {
	return &FileManager{}
}

// WriteLines writes lines to a file in a thread-safe manner
func (fm *FileManager) WriteLines(filePath string, lines []string) error {
	fm.mutex.Lock()
	defer fm.mutex.Unlock()

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filePath, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	for i, line := range lines {
		if i > 0 {
			writer.WriteString("\n")
		}
		writer.WriteString(line)
	}

	return nil
}

// ReadLines reads lines from a file
func (fm *FileManager) ReadLines(filePath string) ([]string, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)

	const maxCapacity = 512 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file %s: %w", filePath, err)
	}

	return lines, nil
}

// AppendLine appends a line to a file
func (fm *FileManager) AppendLine(filePath string, line string) error {
	fm.mutex.Lock()
	defer fm.mutex.Unlock()

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file for append %s: %w", filePath, err)
	}
	defer file.Close()

	_, err = file.WriteString(line + "\n")
	if err != nil {
		return fmt.Errorf("failed to write to file %s: %w", filePath, err)
	}

	return file.Sync()
}

// RemoveLineFromFile removes a specific line from a file
func (fm *FileManager) RemoveLineFromFile(filePath string, lineToRemove string) error {
	lines, err := fm.ReadLines(filePath)
	if err != nil {
		return err
	}

	var newLines []string
	for _, line := range lines {
		if line != lineToRemove {
			newLines = append(newLines, line)
		}
	}

	return fm.WriteLines(filePath, newLines)
}
