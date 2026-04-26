package notifier

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
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
	Season     int     `json:"season"`
	PosterPath *string `json:"posterPath"`
	Note       *string `json:"note"`
}

// SubscriptionResultNotification 订阅结果通知数据
type SubscriptionResultNotification struct {
	TelegramID     int64   `json:"telegramId"`
	SubscriptionID string  `json:"subscriptionId"`
	UserName       string  `json:"userName"`
	Type           string  `json:"type"`
	Name           string  `json:"name"`
	TmdbID         string  `json:"tmdbId"`
	Season         int     `json:"season"`
	PosterPath     *string `json:"posterPath"`
	Status         string  `json:"status"`
	RejectReason   string  `json:"rejectReason,omitempty"`
	ReviewedAt     *string `json:"reviewedAt,omitempty"`
	IngestedAt     *string `json:"ingestedAt,omitempty"`
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
//
// 出于"通知内容长期落 Telegram 服务器"的考虑，admin 通知载荷不再包含
// Email / StripeSessionID / StripePaymentIntentID 等可定位单笔支付的强标识；
// 运维需要追溯单笔支付，请走后台审计页通过 paymentId / userId 查询。
type PaymentSuccessNotification struct {
	PaymentID    string  `json:"paymentId"`
	UserID       string  `json:"userId"`
	UserName     string  `json:"userName"`
	PlanID       string  `json:"planId"`
	PlanName     string  `json:"planName"`
	Amount       int64   `json:"amount"`
	Currency     string  `json:"currency"`
	Days         int     `json:"days"`
	OldExpiresAt *string `json:"oldExpiresAt"`
	NewExpiresAt string  `json:"newExpiresAt"`
}

// BotNotifier Bot 通知客户端
type BotNotifier struct {
	botURL string
	secret string
	client *http.Client
}

// NewBotNotifier 创建 BotNotifier
func NewBotNotifier() *BotNotifier {
	configService := configpkg.NewConfigService()
	notifier := &BotNotifier{
		secret: os.Getenv("INTERNAL_API_SECRET"),
		botURL: strings.TrimRight(configService.GetString("BOT_NOTIFY_URL"), "/"),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
	return notifier
}

var (
	sharedBotNotifierOnce    sync.Once
	sharedBotNotifier        *BotNotifier
)

// GetSharedBotNotifier 返回进程内共享的 BotNotifier 单例。
func GetSharedBotNotifier() *BotNotifier {
	sharedBotNotifierOnce.Do(func() {
		sharedBotNotifier = NewBotNotifier()
	})
	return sharedBotNotifier
}

func (n *BotNotifier) refreshConfig() {
	configService := configpkg.NewConfigService()
	n.botURL = strings.TrimRight(configService.GetString("BOT_NOTIFY_URL"), "/")
}

// IsConfigured 检查 Bot 通知配置
func (n *BotNotifier) IsConfigured() bool {
	return n.botURL != ""
}

func (n *BotNotifier) post(path string, data interface{}) {
	if !n.IsConfigured() {
		return
	}

	body, err := json.Marshal(data)
	if err != nil {
		log.Printf("[BotNotifier] 序列化请求失败 endpoint=%s err=%v", path, err)
		return
	}

	req, err := http.NewRequest("POST", n.botURL+path, bytes.NewBuffer(body))
	if err != nil {
		log.Printf("[BotNotifier] 创建请求失败 endpoint=%s err=%v", path, err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", n.secret)

	resp, err := n.client.Do(req)
	if err != nil {
		log.Printf("[BotNotifier] 请求发送失败 endpoint=%s err=%v", path, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[BotNotifier] 请求失败 endpoint=%s status=%d", path, resp.StatusCode)
	}
}

// NotifyNewSubscription 通知 Bot 有新的订阅请求（fire-and-forget）
func (n *BotNotifier) NotifyNewSubscription(data SubscriptionNotification) {
	n.post("/notify/subscription", data)
}

// NotifyNewRegistration 通知 Bot 有新用户注册（fire-and-forget）
func (n *BotNotifier) NotifyNewRegistration(data RegistrationNotification) {
	n.post("/notify/registration", data)
}

// NotifyPaymentSuccess 通知 Bot 有新的支付成功记录（fire-and-forget）
func (n *BotNotifier) NotifyPaymentSuccess(data PaymentSuccessNotification) {
	n.post("/notify/payment", data)
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
	n.post("/notify/ranking", data)
}

// NotifySubscriptionApproved 通知用户订阅已审核通过。
func (n *BotNotifier) NotifySubscriptionApproved(data SubscriptionResultNotification) {
	n.post("/notify/subscription-result", data)
}

// NotifySubscriptionRejected 通知用户订阅已拒绝。
func (n *BotNotifier) NotifySubscriptionRejected(data SubscriptionResultNotification) {
	n.post("/notify/subscription-result", data)
}

// NotifySubscriptionIngested 通知用户订阅已入库。
func (n *BotNotifier) NotifySubscriptionIngested(data SubscriptionResultNotification) {
	n.post("/notify/subscription-result", data)
}
