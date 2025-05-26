package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"linkedin-crawler/internal/models"
	"linkedin-crawler/internal/storage"
)

// QueryService handles LinkedIn profile queries
type QueryService struct {
	tokenManager     *TokenManager
	profileExtractor *ProfileExtractor
	tokenStorage     *storage.TokenStorage
}

// NewQueryService creates a new QueryService instance
func NewQueryService() *QueryService {
	return &QueryService{
		tokenManager:     &TokenManager{},
		profileExtractor: NewProfileExtractor(),
		tokenStorage:     storage.NewTokenStorage(),
	}
}

// QueryProfileWithRetryLogic queries LinkedIn profile with retry logic and token switching
func (qs *QueryService) QueryProfileWithRetryLogic(lc *models.LinkedInCrawler, ctx context.Context, email string) (bool, []byte, int, error) {
	if qs.tokenManager.AreAllTokensFailed(lc) {
		return false, nil, 0, fmt.Errorf("all tokens have failed")
	}

	// Wait for rate limit token (requests per second max)
	select {
	case <-lc.RequestChan:
		// Got permission to make request
	case <-ctx.Done():
		return false, nil, 0, ctx.Err()
	}

	// Acquire semaphore to limit concurrent requests
	if err := lc.RequestSemaphore.Acquire(ctx, 1); err != nil {
		return false, nil, 0, err
	}

	// Track active requests
	atomic.AddInt32(&lc.ActiveRequests, 1)

	defer func() {
		// Release semaphore and decrement active requests when done
		lc.RequestSemaphore.Release(1)
		atomic.AddInt32(&lc.ActiveRequests, -1)
	}()

	// Thử với token đầu tiên
	token := qs.tokenManager.GetToken(lc)
	hasProfile, body, statusCode, err := qs.doQueryProfile(lc, ctx, email, token)

	// Xử lý logic token switching đặc biệt cho 429
	if statusCode == 429 {
		activeTokenCount := qs.tokenManager.GetValidTokenCount(lc)

		if activeTokenCount > 1 {
			// Có nhiều hơn 1 token active → Chuyển sang token khác
			fmt.Printf("🔄 Token bị 429, chuyển sang token khác (có %d tokens active)\n", activeTokenCount)

			// Đánh dấu token hiện tại là tạm thời invalid (không xóa khỏi file)
			qs.tokenManager.MarkTokenAsInvalid(lc, token)

			// Thử với token khác
			newToken := qs.tokenManager.GetToken(lc)
			if newToken != "" && newToken != token {
				hasProfile, body, statusCode, err = qs.doQueryProfile(lc, ctx, email, newToken)
			}
		} else {
			time.Sleep(1 * time.Second)
			// Thử lại với cùng token
			hasProfile, body, statusCode, err = qs.doQueryProfile(lc, ctx, email, token)
		}
	} else if statusCode == 401 || statusCode == 424 {
		// Xóa token không hợp lệ khỏi file
		qs.tokenManager.MarkTokenAsInvalid(lc, token)

		if err := qs.tokenStorage.RemoveTokenFromFile(lc.TokensFilePath, token); err != nil {
			fmt.Printf("⚠️ Không thể xóa token khỏi file: %v\n", err)
		} else {
			fmt.Printf("🗑️ Đã xóa token không hợp lệ khỏi file (status: %d)\n", statusCode)
		}

		// Kiểm tra xem còn token hợp lệ không
		if qs.tokenManager.CheckIfAllTokensInvalid(lc) {
			return false, nil, statusCode, fmt.Errorf("all tokens have failed")
		}

		// Thử với token khác
		newToken := qs.tokenManager.GetToken(lc)
		if newToken != "" {
			hasProfile, body, statusCode, err = qs.doQueryProfile(lc, ctx, email, newToken)
		}
	}

	return hasProfile, body, statusCode, err
}

// DoQueryProfile performs the actual HTTP request to LinkedIn API (exported method)
func (qs *QueryService) DoQueryProfile(lc *models.LinkedInCrawler, ctx context.Context, email, token string) (bool, []byte, int, error) {
	return qs.doQueryProfile(lc, ctx, email, token)
}

// doQueryProfile performs the actual HTTP request to LinkedIn API
func (qs *QueryService) doQueryProfile(lc *models.LinkedInCrawler, ctx context.Context, email, token string) (bool, []byte, int, error) {
	authHeader := "Bearer " + token

	rootCorrelationID := uuid.New().String()
	correlationID := uuid.New().String()
	clientCorrelationID := uuid.New().String()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://eur.loki.delve.office.com/api/v1/linkedin/profiles/full", nil)
	if err != nil {
		return false, nil, 0, err
	}

	q := req.URL.Query()
	q.Add("Smtp", email)
	q.Add("RootCorrelationId", rootCorrelationID)
	q.Add("CorrelationId", correlationID)
	q.Add("ClientCorrelationId", clientCorrelationID)
	q.Add("UserLocale", "en-US")
	q.Add("ExternalPageInstance", "0000-0000-0000-0000-0000")
	q.Add("PersonaType", "User")
	req.URL.RawQuery = q.Encode()

	req.Header.Add("Authorization", authHeader)
	req.Header.Add("X-ClientFeature", "LivePersonaCard")
	req.Header.Add("Accept", "text/plain, application/json, text/json")
	req.Header.Add("X-ClientType", "OwaMail")
	req.Header.Add("X-HostAppCapabilities", "{}")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:57.0) Gecko/20100101 Firefox/57.0")
	req.Header.Add("Connection", "keep-alive")
	req.Header.Add("X-LPCVersion", "1.20210418.1.0")

	resp, err := lc.Client.Do(req)
	if err != nil {
		return false, nil, 0, err
	}
	defer resp.Body.Close()

	statusCode := resp.StatusCode

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return false, nil, statusCode, fmt.Errorf("token authentication failed (401 Unauthorized): %s", resp.Status)
		} else if resp.StatusCode == 424 {
			return false, nil, statusCode, fmt.Errorf("token dependency failed (424 Failed Dependency): %s", resp.Status)
		} else if resp.StatusCode == 429 {
			return false, nil, statusCode, fmt.Errorf("rate limited (429 Too Many Requests): %s", resp.Status)
		} else if resp.StatusCode == 500 {
			return false, nil, statusCode, fmt.Errorf("internal server error (500): %s", resp.Status)
		}
		return false, nil, statusCode, fmt.Errorf("HTTP error: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, nil, statusCode, err
	}

	hasProfile := strings.Contains(string(body), "displayName")

	return hasProfile, body, statusCode, nil
}
