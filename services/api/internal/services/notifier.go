package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// SubscriptionNotification 新订阅通知数据
type SubscriptionNotification struct {
	ID         string  `json:"id"`
	UserName   string  `json:"userName"`
	Type       string  `json:"type"`
	Name       string  `json:"name"`
	TmdbID     string  `json:"tmdbId"`
	PosterPath *string `json:"posterPath"`
	Note       *string `json:"note"`
}

// BotNotifier Bot 通知客户端
type BotNotifier struct {
	botURL string
	secret string
	client *http.Client
}

// NewBotNotifier 创建 BotNotifier
func NewBotNotifier() *BotNotifier {
	return &BotNotifier{
		botURL: strings.TrimRight(os.Getenv("BOT_NOTIFY_URL"), "/"),
		secret: os.Getenv("INTERNAL_API_SECRET"),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// IsConfigured 检查 Bot 通知配置
func (n *BotNotifier) IsConfigured() bool {
	return n.botURL != ""
}

// NotifyNewSubscription 通知 Bot 有新的订阅请求（fire-and-forget）
func (n *BotNotifier) NotifyNewSubscription(data SubscriptionNotification) {
	if !n.IsConfigured() {
		return
	}

	body, err := json.Marshal(data)
	if err != nil {
		fmt.Printf("Bot 通知失败：序列化请求失败: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", n.botURL+"/notify/subscription", bytes.NewBuffer(body))
	if err != nil {
		fmt.Printf("Bot 通知失败：创建请求失败: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", n.secret)

	resp, err := n.client.Do(req)
	if err != nil {
		fmt.Printf("Bot 通知失败：请求发送失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Bot 通知失败：状态码 %d\n", resp.StatusCode)
	}
}
