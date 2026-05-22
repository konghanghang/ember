package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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

// SubscriptionAdminDelivery 是 Bot 返回的单条管理员审批消息投递结果。
type SubscriptionAdminDelivery struct {
	AdminTelegramID int64   `json:"adminTelegramId"`
	ChatID          int64   `json:"chatId"`
	MessageID       *int64  `json:"messageId,omitempty"`
	HasPhoto        bool    `json:"hasPhoto"`
	DeliveryStatus  string  `json:"deliveryStatus"`
	FailureReason   *string `json:"failureReason,omitempty"`
}

// SubscriptionAdminNotificationRef 是 API 请求 Bot 同步管理员消息时传入的消息引用。
type SubscriptionAdminNotificationRef struct {
	ID              string `json:"id"`
	AdminTelegramID int64  `json:"adminTelegramId"`
	ChatID          int64  `json:"chatId"`
	MessageID       int64  `json:"messageId"`
	HasPhoto        bool   `json:"hasPhoto"`
}

// SubscriptionAdminSyncRequest 描述一次订阅审批结果对管理员消息的批量同步。
type SubscriptionAdminSyncRequest struct {
	SubscriptionID string                             `json:"subscriptionId"`
	UserName       string                             `json:"userName"`
	Type           string                             `json:"type"`
	Name           string                             `json:"name"`
	TmdbID         string                             `json:"tmdbId"`
	Season         int                                `json:"season"`
	PosterPath     *string                            `json:"posterPath"`
	Note           string                             `json:"note,omitempty"`
	Status         string                             `json:"status"`
	RejectReason   string                             `json:"rejectReason,omitempty"`
	ReviewedAt     *string                            `json:"reviewedAt,omitempty"`
	Notifications  []SubscriptionAdminNotificationRef `json:"notifications"`
}

// SubscriptionAdminSyncResult 是 Bot 对单条管理员审批消息的编辑结果。
type SubscriptionAdminSyncResult struct {
	ID             string  `json:"id"`
	DeliveryStatus string  `json:"deliveryStatus"`
	FailureReason  *string `json:"failureReason,omitempty"`
}

type subscriptionNotifyResponse struct {
	Deliveries []SubscriptionAdminDelivery `json:"deliveries"`
}

type subscriptionAdminSyncResponse struct {
	Results []SubscriptionAdminSyncResult `json:"results"`
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
	botURL          string
	secret          string
	client          *http.Client
	mu              sync.RWMutex
	lastRefreshedAt time.Time
	requestSeq      atomic.Uint64
}

const botNotifierRefreshTTL = 30 * time.Second

// NewBotNotifier 创建 BotNotifier
func NewBotNotifier() *BotNotifier {
	notifier := &BotNotifier{
		secret: os.Getenv("INTERNAL_API_SECRET"),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
	notifier.refreshConfig(true)
	return notifier
}

var (
	sharedBotNotifierOnce sync.Once
	sharedBotNotifier     *BotNotifier
)

// GetSharedBotNotifier 返回进程内共享的 BotNotifier 单例。
func GetSharedBotNotifier() *BotNotifier {
	sharedBotNotifierOnce.Do(func() {
		sharedBotNotifier = NewBotNotifier()
	})
	return sharedBotNotifier
}

func (n *BotNotifier) refreshConfig(force bool) {
	n.mu.RLock()
	shouldSkip := !force && !n.lastRefreshedAt.IsZero() && time.Since(n.lastRefreshedAt) < botNotifierRefreshTTL
	n.mu.RUnlock()
	if shouldSkip {
		return
	}

	configService := configpkg.NewConfigService()
	botURL := strings.TrimRight(configService.GetString("BOT_NOTIFY_URL"), "/")

	n.mu.Lock()
	n.botURL = botURL
	n.lastRefreshedAt = time.Now().UTC()
	n.mu.Unlock()
}

func (n *BotNotifier) Reload() {
	n.refreshConfig(true)
}

// IsConfigured 检查 Bot 通知配置
func (n *BotNotifier) IsConfigured() bool {
	n.refreshConfig(false)
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.botURL != ""
}

func notifierEventName(path string) string {
	event := strings.Trim(path, "/")
	if event == "" {
		return "unknown"
	}
	return strings.ReplaceAll(event, "/", ".")
}

func (n *BotNotifier) post(path string, data interface{}) {
	if !n.IsConfigured() {
		return
	}

	n.mu.RLock()
	botURL := n.botURL
	n.mu.RUnlock()

	body, err := json.Marshal(data)
	if err != nil {
		log.Printf("[BotNotifier] 序列化请求失败 endpoint=%s event=%s err=%v", path, notifierEventName(path), err)
		return
	}

	requestID := fmt.Sprintf("bot-notify-%d", n.requestSeq.Add(1))
	payloadSize := len(body)
	start := time.Now()

	req, err := http.NewRequest("POST", botURL+path, bytes.NewBuffer(body))
	if err != nil {
		log.Printf(
			"[BotNotifier] 创建请求失败 endpoint=%s event=%s payloadSize=%d requestId=%s err=%v",
			path, notifierEventName(path), payloadSize, requestID, err,
		)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", n.secret)
	req.Header.Set("X-Request-Id", requestID)

	resp, err := n.client.Do(req)
	if err != nil {
		log.Printf(
			"[BotNotifier] 请求发送失败 endpoint=%s event=%s payloadSize=%d requestId=%s latency=%s err=%v",
			path, notifierEventName(path), payloadSize, requestID, time.Since(start), err,
		)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf(
			"[BotNotifier] 请求失败 endpoint=%s event=%s payloadSize=%d requestId=%s status=%d latency=%s",
			path, notifierEventName(path), payloadSize, requestID, resp.StatusCode, time.Since(start),
		)
	}
}

func (n *BotNotifier) postAndDecode(path string, data interface{}, out interface{}) error {
	if !n.IsConfigured() {
		return nil
	}

	n.mu.RLock()
	botURL := n.botURL
	n.mu.RUnlock()

	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化请求失败 endpoint=%s event=%s: %w", path, notifierEventName(path), err)
	}

	requestID := fmt.Sprintf("bot-notify-%d", n.requestSeq.Add(1))
	payloadSize := len(body)
	start := time.Now()

	req, err := http.NewRequest("POST", botURL+path, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("创建请求失败 endpoint=%s event=%s payloadSize=%d requestId=%s: %w",
			path, notifierEventName(path), payloadSize, requestID, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", n.secret)
	req.Header.Set("X-Request-Id", requestID)

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求发送失败 endpoint=%s event=%s payloadSize=%d requestId=%s latency=%s: %w",
			path, notifierEventName(path), payloadSize, requestID, time.Since(start), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("请求失败 endpoint=%s event=%s payloadSize=%d requestId=%s status=%d latency=%s body=%s",
			path, notifierEventName(path), payloadSize, requestID, resp.StatusCode, time.Since(start), strings.TrimSpace(string(respBody)))
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("解析响应失败 endpoint=%s event=%s requestId=%s: %w", path, notifierEventName(path), requestID, err)
	}
	return nil
}

// NotifyNewSubscription 通知 Bot 有新的订阅请求（fire-and-forget）
func (n *BotNotifier) NotifyNewSubscription(data SubscriptionNotification) {
	n.post("/notify/subscription", data)
}

// NotifyNewSubscriptionWithDeliveries 通知 Bot 发送订阅审批消息并返回每条管理员消息的投递引用。
func (n *BotNotifier) NotifyNewSubscriptionWithDeliveries(data SubscriptionNotification) ([]SubscriptionAdminDelivery, error) {
	var resp subscriptionNotifyResponse
	if err := n.postAndDecode("/notify/subscription", data, &resp); err != nil {
		return nil, err
	}
	return resp.Deliveries, nil
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

// SyncSubscriptionAdminMessages 请求 Bot 批量同步订阅管理员审批消息状态。
func (n *BotNotifier) SyncSubscriptionAdminMessages(data SubscriptionAdminSyncRequest) ([]SubscriptionAdminSyncResult, error) {
	var resp subscriptionAdminSyncResponse
	if err := n.postAndDecode("/notify/subscription-admin-sync", data, &resp); err != nil {
		return nil, err
	}
	return resp.Results, nil
}
