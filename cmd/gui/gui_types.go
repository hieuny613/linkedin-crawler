package main

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"linkedin-crawler/internal/models"
)

// ConfigTab handles configuration settings
type ConfigTab struct {
	gui *CrawlerGUI

	// Form fields
	maxConcurrency *widget.Entry
	requestsPerSec *widget.Entry
	requestTimeout *widget.Entry
	minTokens      *widget.Entry
	maxTokens      *widget.Entry
	sleepDuration  *widget.Entry

	// Buttons
	saveBtn  *widget.Button
	resetBtn *widget.Button

	// Current config
	config models.Config
}

// ControlTab handles crawler execution control
type ControlTab struct {
	gui *CrawlerGUI

	// Control buttons
	startBtn  *widget.Button
	stopBtn   *widget.Button
	pauseBtn  *widget.Button
	resumeBtn *widget.Button

	// Progress
	progressBar   *widget.ProgressBar
	progressLabel *widget.Label

	// Real-time stats
	processedLabel *widget.Label
	successLabel   *widget.Label
	failedLabel    *widget.Label
	tokensLabel    *widget.Label
	rateLabel      *widget.Label

	// Status
	statusLabel *widget.Label
	timeLabel   *widget.Label

	// Update ticker
	updateTicker *time.Ticker

	// Stats
	startTime       time.Time
	totalEmails     int
	processedEmails int

	// Activity log
	activityText   *widget.RichText
	activityBuffer []string // NEW: Buffer for activity history
}

// ResultsTab shows crawling results
type ResultsTab struct {
	gui *CrawlerGUI

	// Results table
	resultsTable *widget.Table
	results      []CrawlerResult

	// Controls
	refreshBtn  *widget.Button
	exportBtn   *widget.Button
	clearBtn    *widget.Button
	filterEntry *widget.Entry

	// Stats summary
	summaryCard *widget.Card
}

// LogsTab shows real-time logs
type LogsTab struct {
	gui *CrawlerGUI

	// Log display
	logText   *widget.RichText
	logScroll *container.Scroll

	// Controls
	clearBtn   *widget.Button
	saveBtn    *widget.Button
	autoScroll *widget.Check

	// Log levels
	levelSelect *widget.Select

	// Log buffer
	logBuffer []string
	maxLogs   int
}

// CrawlerResult represents a single crawling result
type CrawlerResult struct {
	Email       string
	Name        string
	LinkedInURL string
	Location    string
	Connections string
	Status      string
	Timestamp   time.Time
}

// EmailStatus represents the processing status of an email
type EmailStatus string

const (
	StatusPending EmailStatus = "pending"
	StatusSuccess EmailStatus = "success"
	StatusFailed  EmailStatus = "failed"
)

// GUISettings represents GUI-specific settings
type GUISettings struct {
	WindowWidth  int               `json:"window_width"`
	WindowHeight int               `json:"window_height"`
	WindowX      int               `json:"window_x"`
	WindowY      int               `json:"window_y"`
	Theme        string            `json:"theme"` // "light", "dark", "auto"
	LastTab      int               `json:"last_tab"`
	AutoRefresh  bool              `json:"auto_refresh"`
	LogLevel     string            `json:"log_level"`
	ColumnWidths map[string]int    `json:"column_widths"`
	SortSettings map[string]string `json:"sort_settings"`
}

// ValidationError represents a form validation error
type ValidationError struct {
	Field   string
	Message string
	Value   string
}

func (ve ValidationError) Error() string {
	return ve.Message
}

// ProgressInfo represents progress information for the UI
type ProgressInfo struct {
	Total     int
	Processed int
	Success   int
	Failed    int
	HasInfo   int
	NoInfo    int
	Rate      float64
	Elapsed   time.Duration
	Estimated time.Duration
}

// StatsInfo represents statistics for display
type StatsInfo struct {
	TotalEmails    int
	PendingEmails  int
	SuccessEmails  int
	FailedEmails   int
	HasInfoEmails  int
	NoInfoEmails   int
	TotalAccounts  int
	UsedAccounts   int
	ValidTokens    int
	TotalTokens    int
	ProcessingRate float64
	SuccessRate    float64
	LastUpdated    time.Time
}

// FileImportResult represents the result of a file import operation
type FileImportResult struct {
	TotalLines     int
	ImportedItems  int
	SkippedItems   int
	ErrorItems     int
	Errors         []string
	DuplicateItems int
}

// ExportOptions represents options for exporting data
type ExportOptions struct {
	Format       string // "csv", "json", "txt"
	IncludeAll   bool
	DateRange    DateRange
	StatusFilter []EmailStatus
}

// DateRange represents a date range for filtering
type DateRange struct {
	Start time.Time
	End   time.Time
}

// NotificationLevel represents the level of notifications
type NotificationLevel int

const (
	NotificationInfo NotificationLevel = iota
	NotificationWarning
	NotificationError
	NotificationSuccess
)

// Notification represents a system notification
type Notification struct {
	Level     NotificationLevel
	Title     string
	Message   string
	Timestamp time.Time
	Actions   []NotificationAction
}

// NotificationAction represents an action that can be taken from a notification
type NotificationAction struct {
	Label    string
	Callback func()
}

// TabContent interface for tab content creation
type TabContent interface {
	CreateContent() fyne.CanvasObject
	GetTitle() string
	GetIcon() fyne.Resource
	OnShow()
	OnHide()
	Refresh()
}

// DataValidator interface for data validation
type DataValidator interface {
	Validate(data interface{}) []ValidationError
}

// FileHandler interface for file operations
type FileHandler interface {
	CanHandle(filename string) bool
	Import(filename string) (FileImportResult, error)
	Export(filename string, data interface{}, options ExportOptions) error
}

// Theme constants
const (
	ThemeLight = "light"
	ThemeDark  = "dark"
	ThemeAuto  = "auto"
)

// Default GUI settings
var DefaultGUISettings = GUISettings{
	WindowWidth:  1400,
	WindowHeight: 900,
	WindowX:      -1, // Center
	WindowY:      -1, // Center
	Theme:        ThemeAuto,
	LastTab:      0,
	AutoRefresh:  true,
	LogLevel:     "info",
	ColumnWidths: map[string]int{
		"email":       200,
		"name":        150,
		"linkedin":    250,
		"location":    150,
		"connections": 100,
		"status":      100,
	},
	SortSettings: map[string]string{
		"results":  "timestamp_desc",
		"emails":   "email_asc",
		"accounts": "email_asc",
	},
}

// Common validation patterns
var (
	EmailPattern    = `^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`
	PasswordPattern = `^.{6,}$` // Minimum 6 characters
	URLPattern      = `^https?://.*$`
	NumberPattern   = `^\d+$`
	FloatPattern    = `^\d+\.?\d*$`
)

// Color constants for UI elements
var (
	ColorSuccess = "#4CAF50"
	ColorWarning = "#FF9800"
	ColorError   = "#F44336"
	ColorInfo    = "#2196F3"
	ColorPending = "#9E9E9E"
)

// Icon constants (would be loaded from resources)
var (
	IconStart    fyne.Resource
	IconStop     fyne.Resource
	IconPause    fyne.Resource
	IconResume   fyne.Resource
	IconRefresh  fyne.Resource
	IconExport   fyne.Resource
	IconImport   fyne.Resource
	IconSettings fyne.Resource
	IconAccount  fyne.Resource
	IconEmail    fyne.Resource
	IconLinkedIn fyne.Resource
)

// Animation duration constants
const (
	AnimationDurationShort  = 200 * time.Millisecond
	AnimationDurationMedium = 500 * time.Millisecond
	AnimationDurationLong   = 1000 * time.Millisecond
)

// Refresh intervals
const (
	RefreshIntervalFast   = 1 * time.Second
	RefreshIntervalMedium = 5 * time.Second
	RefreshIntervalSlow   = 30 * time.Second
)

// File size limits
const (
	MaxFileSize     = 10 * 1024 * 1024 // 10MB
	MaxLogEntries   = 10000
	MaxResultsShow  = 5000
	MaxEmailsImport = 50000
)

// Helper function to create a progress info from stats
func NewProgressInfo(stats StatsInfo) ProgressInfo {
	total := stats.TotalEmails
	processed := stats.SuccessEmails + stats.FailedEmails

	var rate float64
	if stats.ProcessingRate > 0 {
		rate = stats.ProcessingRate
	}

	var estimated time.Duration
	if rate > 0 && total > processed {
		remaining := total - processed
		estimated = time.Duration(float64(remaining)/rate) * time.Second
	}

	return ProgressInfo{
		Total:     total,
		Processed: processed,
		Success:   stats.SuccessEmails,
		Failed:    stats.FailedEmails,
		HasInfo:   stats.HasInfoEmails,
		NoInfo:    stats.NoInfoEmails,
		Rate:      rate,
		Estimated: estimated,
	}
}
