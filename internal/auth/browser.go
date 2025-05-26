package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// BrowserManager handles Chrome browser automation
type BrowserManager struct{}

// NewBrowserManager creates a new BrowserManager instance
func NewBrowserManager() *BrowserManager {
	return &BrowserManager{}
}

// CreateBrowserContext creates and configures a Chrome browser context
func (bm *BrowserManager) CreateBrowserContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-plugins", true),
		chromedp.Flag("disable-images", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)

	// Enable network events
	if err := chromedp.Run(browserCtx, network.Enable()); err != nil {
		cancel()
		browserCancel()
		return nil, nil, fmt.Errorf("không enable được network events: %v", err)
	}

	// Return a combined cancel function
	combinedCancel := func() {
		browserCancel()
		cancel()
	}

	return browserCtx, combinedCancel, nil
}

// HandleStaySignedInPrompt handles the "Stay signed in?" prompt
func (bm *BrowserManager) HandleStaySignedInPrompt(ctx context.Context, promptName string) error {
	var exists bool
	err := chromedp.Evaluate(`document.querySelector('input[type="submit"][value="Yes"]') !== null`, &exists).Do(ctx)
	if err != nil {
		return err
	}

	if exists {
		fmt.Printf("✅ Tìm thấy prompt 'Stay signed in?' (%s), clicking Yes...\n", promptName)
		err = chromedp.Click(`input[type="submit"][value="Yes"]`, chromedp.ByQuery).Do(ctx)
		if err != nil {
			return err
		}
		chromedp.Sleep(10 * time.Second).Do(ctx)
		fmt.Printf("✅ Đã click Yes cho prompt 'Stay signed in?' (%s)\n", promptName)
	}

	return nil
}

// ClickChatButton finds and clicks the chat button
func (bm *BrowserManager) ClickChatButton(ctx context.Context) error {
	selectors := []string{
		`button[aria-label="Trò chuyện"]`,
		`button[aria-label="Chat"]`,
		`button[aria-label*="chat"]`,
		`button[aria-label*="Chat"]`,
		`//button[contains(., "Trò chuyện")]`,
		`//button[contains(., "Chat")]`,
		`button[data-tid*="chat"]`,
		`//button[.//svg[contains(@viewBox, "24 24") and .//path[contains(@d, "M12 2a10 10 0 1 1-4.59 18.89")]]]`,
	}

	for _, selector := range selectors {
		var found bool
		var err error

		if strings.HasPrefix(selector, "//") {
			err = chromedp.Run(ctx,
				chromedp.ActionFunc(func(ctx context.Context) error {
					var exists bool
					evalScript := fmt.Sprintf(`document.evaluate('%s', document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue !== null`, selector)
					chromedp.Evaluate(evalScript, &exists).Do(ctx)
					found = exists
					return nil
				}),
			)
		} else {
			err = chromedp.Run(ctx,
				chromedp.ActionFunc(func(ctx context.Context) error {
					var exists bool
					evalScript := fmt.Sprintf(`document.querySelector('%s') !== null`, selector)
					chromedp.Evaluate(evalScript, &exists).Do(ctx)
					found = exists
					return nil
				}),
			)
		}

		if err == nil && found {
			var clickErr error
			if strings.HasPrefix(selector, "//") {
				clickErr = chromedp.Run(ctx, chromedp.Click(selector, chromedp.BySearch))
			} else {
				clickErr = chromedp.Run(ctx, chromedp.Click(selector, chromedp.ByQuery))
			}

			if clickErr == nil {
				chromedp.Sleep(2 * time.Second).Do(ctx)

				if strings.HasPrefix(selector, "//") {
					chromedp.Run(ctx, chromedp.Click(selector, chromedp.BySearch))
				} else {
					chromedp.Run(ctx, chromedp.Click(selector, chromedp.ByQuery))
				}

				return nil
			}
		}
	}

	return fmt.Errorf("không thể tìm thấy hoặc click vào nút Chat")
}

// ClickChatConversation finds and clicks the chat conversation
func (bm *BrowserManager) ClickChatConversation(ctx context.Context) error {
	selectors := []string{
		`//*[starts-with(@id, "chat-topic-person-8:orgid:")]`,
		`//*[contains(@id, "chat-topic-person")]`,
		`//*[contains(@class, "chat-topic")]`,
		`//div[contains(@role, "listitem")]//div[contains(@class, "chat")]`,
		`//div[contains(@data-tid, "chat")]`,
		`//div[contains(@aria-label, "chat")]`,
	}

	for _, selector := range selectors {
		err := chromedp.Run(ctx,
			chromedp.WaitVisible(selector, chromedp.BySearch),
			chromedp.Click(selector, chromedp.BySearch),
		)

		if err == nil {
			chromedp.Sleep(2 * time.Second).Do(ctx)
			chromedp.Run(ctx, chromedp.Click(selector, chromedp.BySearch))
			return nil
		}
	}

	return fmt.Errorf("không thể tìm thấy hoặc click vào chat conversation")
}
