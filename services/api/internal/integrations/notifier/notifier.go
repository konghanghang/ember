package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
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

// RegistrationNotification 新用户注册通知数据
type RegistrationNotification struct {
	ID               string  `json:"id"`
	UserName         string  `json:"userName"`
	Email            string  `json:"email"`
	EmbyID           string  `json:"embyId"`
	RegistrationMode string  `json:"registrationMode"`
	ExpiresAt        *string `json:"expiresAt"`
}

// PaymentSuccessNotification 支付成功通知数据
type PaymentSuccessNotification struct {
	PaymentID             string  `json:"paymentId"`
	UserID                string  `json:"userId"`
	UserName              string  `json:"userName"`
	Email                 string  `json:"email"`
	PlanID                string  `json:"planId"`
	PlanName              string  `json:"planName"`
	Amount                int64   `json:"amount"`
	Currency              string  `json:"currency"`
	Days                  int     `json:"days"`
	OldExpiresAt          *string `json:"oldExpiresAt"`
	NewExpiresAt          string  `json:"newExpiresAt"`
	StripeSessionID       string  `json:"stripeSessionId"`
	StripePaymentIntentID string  `json:"stripePaymentIntentId"`
}

// BotNotifier Bot 通知客户端
type BotNotifier struct {
	botURL string
	secret string
	client *http.Client
}

// NewBotNotifier 创建 BotNotifier
func NewBotNotifier() *BotNotifier {
	notifier := &BotNotifier{
		secret: os.Getenv("INTERNAL_API_SECRET"),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
	notifier.refreshConfig()
	return notifier
}

func (n *BotNotifier) refreshConfig() {
	configService := configpkg.NewConfigService()
	n.botURL = strings.TrimRight(configService.GetString("BOT_NOTIFY_URL"), "/")
}

// IsConfigured 检查 Bot 通知配置
func (n *BotNotifier) IsConfigured() bool {
	n.refreshConfig()
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

// NotifyNewRegistration 通知 Bot 有新用户注册（fire-and-forget）
func (n *BotNotifier) NotifyNewRegistration(data RegistrationNotification) {
	if !n.IsConfigured() {
		return
	}

	body, err := json.Marshal(data)
	if err != nil {
		fmt.Printf("Bot 通知失败：序列化请求失败: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", n.botURL+"/notify/registration", bytes.NewBuffer(body))
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

// NotifyPaymentSuccess 通知 Bot 有新的支付成功记录（fire-and-forget）
func (n *BotNotifier) NotifyPaymentSuccess(data PaymentSuccessNotification) {
	if !n.IsConfigured() {
		return
	}

	body, err := json.Marshal(data)
	if err != nil {
		fmt.Printf("Bot 通知失败：序列化请求失败: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", n.botURL+"/notify/payment", bytes.NewBuffer(body))
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

// RankingNotification 排行榜推送数据
type RankingNotification struct {
	Period        string              `json:"period"`        // "daily" 或 "weekly"
	PeriodStart   string              `json:"periodStart"`   // "2026-02-14"
	PeriodEnd     string              `json:"periodEnd"`     // "2026-02-14"
	CutoffAt      string              `json:"cutoffAt"`      // "20:00" (阶段榜截止时间，可选)
	TotalDuration int64               `json:"totalDuration"` // 秒
	Movies        []RankingItemNotify `json:"movies"`
	Episodes      []RankingItemNotify `json:"episodes"`
}

// RankingItemNotify 排行条目
type RankingItemNotify struct {
	Rank     int    `json:"rank"`
	Name     string `json:"name"`
	Duration int64  `json:"duration"` // 秒
	Count    int    `json:"count"`
}

// NotifyRanking 通知 Bot 发送排行榜到 Telegram 群组（fire-and-forget）
func (n *BotNotifier) NotifyRanking(data RankingNotification) {
	if !n.IsConfigured() {
		return
	}

	body, err := json.Marshal(data)
	if err != nil {
		fmt.Printf("Bot 通知失败：序列化请求失败: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", n.botURL+"/notify/ranking", bytes.NewBuffer(body))
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
