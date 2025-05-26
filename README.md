# LinkedIn Auto Crawler

A sophisticated LinkedIn profile crawler that extracts LinkedIn information using Microsoft Teams authentication tokens.

## 🚀 Features

- **Automated Token Extraction**: Automatically logs into Microsoft Teams accounts to extract authentication tokens
- **Concurrent Processing**: Multi-threaded email processing with configurable concurrency
- **Token Rotation**: Smart token management with automatic rotation and validation
- **Retry Logic**: Advanced retry mechanism for failed requests
- **State Persistence**: Resumes processing from where it left off after interruption
- **Rate Limiting**: Built-in rate limiting to avoid being blocked
- **Progress Tracking**: Real-time progress display with detailed statistics
- **Memory Efficient**: Optimized memory usage with proper cleanup

## 📁 Project Structure

```
linkedin-crawler/
├── cmd/crawler/main.go                ✅ Entry point
├── internal/
│   ├── config/config.go              ✅ Configuration
│   ├── models/
│   │   ├── account.go                ✅ Account structures  
│   │   ├── config.go                 ✅ Config structures
│   │   ├── crawler.go                ✅ Crawler structures + methods
│   │   └── profile.go                ✅ Profile structures
│   ├── auth/
│   │   ├── browser.go                ✅ Browser automation
│   │   ├── login.go                  ✅ Login automation  
│   │   └── token_extractor.go        ✅ Token extraction
│   ├── crawler/
│   │   ├── crawler.go                ✅ Core crawler
│   │   ├── profile_extractor.go      ✅ Profile extraction
│   │   ├── query.go                  ✅ LinkedIn API queries
│   │   └── token_manager.go          ✅ Token management
│   ├── orchestrator/
│   │   ├── auto_crawler.go           ✅ Main orchestrator
│   │   ├── batch_processor.go        ✅ Batch processing
│   │   ├── retry_handler.go          ✅ Retry handling
│   │   └── state_manager.go          ✅ State management
│   ├── storage/
│   │   ├── account_storage.go        ✅ Account operations
│   │   ├── email_storage.go          ✅ Email operations
│   │   ├── file_manager.go           ✅ Base file operations
│   │   ├── legacy_functions.go       ✅ Legacy compatibility
│   │   └── token_storage.go          ✅ Token operations
│   └── utils/
│       ├── format.go                 ✅ Formatting utilities
│       └── signal.go                 ✅ Signal handling
├── .gitignore                        ✅ Git ignore rules
├── go.mod                            ✅ Go modules
└── README.md                         ✅ Documentation
```

## 🛠️ Installation

### Prerequisites

- **Go 1.21+**: Download from [golang.org](https://golang.org/)
- **Chrome/Chromium**: Required for browser automation
- **Microsoft Teams Accounts**: Valid accounts for token extraction

### Build from Source

1. **Clone the repository**:
   ```bash
   git clone <repository-url>
   cd linkedin-crawler
   ```

2. **Install dependencies**:
   ```bash
   go mod download
   ```

3. **Build the binary**:
   ```bash
   make build
   # or
   go build -o bin/crawler cmd/crawler/main.go
   ```

## 📋 Configuration

### Required Files

Create these files in the project root:

#### 1. `accounts.txt` - Microsoft Teams Accounts
```
# Format: email|password
user1@company.com|password123
user2@company.com|mypassword456
admin@organization.com|securepass789
```

#### 2. `emails.txt` - Target Emails
```
# One email per line
john.doe@example.com
jane.smith@company.com
ceo@startup.io
developer@techfirm.com
```

#### 3. `tokens.txt` - Authentication Tokens (Auto-generated)
This file is automatically created and managed by the crawler.

### Configuration Options

The crawler uses these default settings (configurable in `internal/config/config.go`):

```go
MaxConcurrency:   50,          // Max concurrent requests
RequestsPerSec:   20.0,        // Rate limit (requests per second)
RequestTimeout:   15 * time.Second,  // Request timeout
MinTokens:        10,          // Minimum tokens before refresh
MaxTokens:        10,          // Maximum tokens to extract per batch
SleepDuration:    1 * time.Minute,   // Sleep before exit
```

## 🚀 Usage

### Quick Start

1. **Prepare your files**:
   ```bash
   # Edit accounts.txt with your Microsoft Teams accounts
   nano accounts.txt
   
   # Edit emails.txt with target emails
   nano emails.txt
   ```

2. **Run the crawler**:
   ```bash
   # Using make
   make run
   
   # Direct execution
   ./bin/crawler
   ```

### Advanced Usage

#### Build Options
```bash
# Development build with checks
make dev-run

# Release build (optimized)
make release

# Clean build
make clean build
```

## 📊 Output Files

### `hit.txt` - LinkedIn Profiles Found
```
email@domain.com|John Doe|https://linkedin.com/in/johndoe|New York, NY|500+
jane@company.com|Jane Smith|https://linkedin.com/in/janesmith|San Francisco, CA|1000+
```

Format: `email|name|linkedin_url|location|connections`

### `crawler.log` - Detailed Logs
Contains detailed execution logs including:
- Token extraction attempts
- API request/response details
- Error messages and retry attempts
- Processing statistics

## 🔧 Architecture

### Core Components

1. **Orchestrator** (`internal/orchestrator/`)
   - Main crawler coordination
   - Batch processing management
   - State persistence and recovery

2. **Authentication** (`internal/auth/`)
   - Browser automation for Teams login
   - Token extraction and validation
   - Account management

3. **Crawler Engine** (`internal/crawler/`)
   - LinkedIn API interaction
   - Token rotation and management
   - Profile data extraction

4. **Storage Layer** (`internal/storage/`)
   - File-based persistence
   - Thread-safe operations
   - Data integrity management

### Processing Flow

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Load Emails   │───▶│  Extract Tokens  │───▶│  Process Batch  │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         ▲                       │                       │
         │                       ▼                       ▼
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│  Update State   │◀───│  Validate Tokens │◀───│   Query LinkedIn │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

## ⚡ Performance

### Benchmarks
- **Throughput**: ~20 requests/second (configurable)
- **Concurrency**: Up to 50 parallel workers
- **Memory Usage**: ~50-100MB typical
- **Token Lifetime**: ~1-2 hours per token

### Optimization Tips

1. **Adjust Concurrency**:
   ```go
   MaxConcurrency: 100,  // Higher for more speed
   RequestsPerSec: 30.0, // Higher rate limit
   ```

2. **Token Management**:
   - Use more Microsoft Teams accounts for better token rotation
   - Monitor token expiration patterns
   - Adjust `MinTokens` and `MaxTokens` based on your needs

## 🛡️ Rate Limiting & Safety

### Built-in Protections
- **Request Rate Limiting**: Configurable requests per second
- **Token Rotation**: Automatic switching when rate limited
- **Graceful Degradation**: Continues with available tokens
- **State Persistence**: Resumes after interruption

### Best Practices
- Use reasonable request rates (10-20 req/sec)
- Monitor for 429 (Too Many Requests) responses
- Rotate Microsoft Teams accounts regularly
- Run during off-peak hours
