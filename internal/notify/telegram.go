package notify

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// SendTelegram sends a message via Telegram Bot API.
func SendTelegram(botToken, chatID, message string) error {
	if botToken == "" || chatID == "" {
		return nil // not configured, skip silently
	}
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm(apiURL, url.Values{
		"chat_id":    {chatID},
		"text":       {message},
		"parse_mode": {"HTML"},
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram API returned %d", resp.StatusCode)
	}
	return nil
}

// FormatAlert formats an alert message for Telegram.
func FormatAlert(hostName, alertType, message string) string {
	emoji := "⚠️"
	switch alertType {
	case "cpu":
		emoji = "🔥"
	case "mem":
		emoji = "💾"
	case "disk":
		emoji = "💿"
	case "offline":
		emoji = "🔴"
	case "expire":
		emoji = "📅"
	}
	return fmt.Sprintf("%s <b>[%s]</b> %s\n%s", emoji, hostName, alertType, message)
}
