package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"

	"linkedin-crawler/internal/models"
)

// LoginService handles Microsoft Teams login process
type LoginService struct {
	browserManager *BrowserManager
}

// NewLoginService creates a new LoginService instance
func NewLoginService() *LoginService {
	return &LoginService{
		browserManager: NewBrowserManager(),
	}
}

// LoginToTeams performs login to Microsoft Teams
func (ls *LoginService) LoginToTeams(ctx context.Context, account models.Account) error {
	loginURL := "https://teams.microsoft.com/"

	fmt.Printf("🔑 Đang xử lý account: %s\n", account.Email)

	err := chromedp.Run(ctx,
		chromedp.Navigate(loginURL),
		chromedp.Sleep(3*time.Second),

		chromedp.WaitVisible(`input[type="email"]`, chromedp.ByQuery),
		chromedp.Clear(`input[type="email"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[type="email"]`, account.Email, chromedp.ByQuery),
		chromedp.Click(`input[type="submit"]`, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),

		chromedp.WaitVisible(`input[type="password"]`, chromedp.ByQuery),
		chromedp.Clear(`input[type="password"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[type="password"]`, account.Password, chromedp.ByQuery),
		chromedp.Click(`input[type="submit"]`, chromedp.ByQuery),
		chromedp.Sleep(5*time.Second),

		chromedp.ActionFunc(func(ctx context.Context) error {
			return ls.browserManager.HandleStaySignedInPrompt(ctx, "sau login")
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			var isChangePasswordPage bool
			chromedp.Evaluate(`document.querySelector('div[data-viewid="22"][data-showidentitybanner="true"]') !== null`, &isChangePasswordPage).Do(ctx)

			if isChangePasswordPage {
				fmt.Println("🔑 Phát hiện trang đổi password!")
				return ls.handleChangePassword(ctx, account)
			}

			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			return ls.browserManager.HandleStaySignedInPrompt(ctx, "sau đổi password")
		}),

		chromedp.Sleep(10*time.Second),

		chromedp.ActionFunc(func(ctx context.Context) error {
			return ls.browserManager.ClickChatButton(ctx)
		}),

		chromedp.Sleep(5*time.Second),

		chromedp.ActionFunc(func(ctx context.Context) error {
			return ls.browserManager.ClickChatConversation(ctx)
		}),

		chromedp.Sleep(5*time.Second),
	)

	return err
}

// handleChangePassword handles password change requirement
func (ls *LoginService) handleChangePassword(ctx context.Context, account models.Account) error {
	fmt.Println("🔑 Phát hiện trang đổi password, đang xử lý...")

	newPassword := account.Password + "d"
	fmt.Printf("Password mới sẽ là: %s\n", newPassword)

	err := chromedp.Run(ctx,
		chromedp.WaitVisible(`#currentPassword`, chromedp.ByID),
		chromedp.Clear(`#currentPassword`, chromedp.ByID),
		chromedp.SendKeys(`#currentPassword`, account.Password, chromedp.ByID),
		chromedp.Sleep(1*time.Second),

		chromedp.WaitVisible(`#newPassword`, chromedp.ByID),
		chromedp.Clear(`#newPassword`, chromedp.ByID),
		chromedp.SendKeys(`#newPassword`, newPassword, chromedp.ByID),
		chromedp.Sleep(1*time.Second),

		chromedp.WaitVisible(`#confirmNewPassword`, chromedp.ByID),
		chromedp.Clear(`#confirmNewPassword`, chromedp.ByID),
		chromedp.SendKeys(`#confirmNewPassword`, newPassword, chromedp.ByID),
		chromedp.Sleep(1*time.Second),

		chromedp.Click(`#idSIButton9`, chromedp.ByID),
		chromedp.Sleep(8*time.Second),
	)

	if err != nil {
		return fmt.Errorf("lỗi khi đổi password: %v", err)
	}

	fmt.Println("✅ Đã đổi password thành công!")

	err = chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var exists bool
			chromedp.Evaluate(`document.querySelector('input[type="submit"][value="Yes"]') !== null`, &exists).Do(ctx)
			if exists {
				fmt.Println("✅ Tìm thấy prompt 'Stay signed in?' sau đổi password, clicking Yes...")
				chromedp.Click(`input[type="submit"][value="Yes"]`, chromedp.ByQuery).Do(ctx)
				chromedp.Sleep(5 * time.Second).Do(ctx)
				fmt.Println("✅ Đã click Yes cho prompt 'Stay signed in?' sau đổi password")
			}
			return nil
		}),
	)

	return err
}
