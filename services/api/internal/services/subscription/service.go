package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/konghang/ember/backend/internal/async"
	"github.com/konghang/ember/backend/internal/common/upstream"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	moviepilotint "github.com/konghang/ember/backend/internal/integrations/moviepilot"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SubscriptionService 订阅服务
type SubscriptionService struct {
	moviepilot subscriptionMoviePilotClient
	notifier   *notifierint.BotNotifier
	httpClient *http.Client
}

type subscriptionMoviePilotClient interface {
	IsConfigured() bool
	CreateSubscription(moviepilotint.SubscribeRequest) error
	SearchMediaCandidates(moviepilotint.SearchMediaRequest) (*moviepilotint.SearchMediaResponse, error)
	DispatchDownloadCandidate(moviepilotint.DownloadCandidateRequest) (*moviepilotint.DownloadCandidateResponse, error)
}

type subscriptionEmbyLookup interface {
	IsConfigured() bool
	GetWithAPIKey(path string, params map[string]string) ([]byte, error)
}

type subscriptionSeriesItem struct {
	ProviderIDs map[string]string `json:"ProviderIds"`
}

type subscriptionSeriesItemsResponse struct {
	Items []subscriptionSeriesItem `json:"Items"`
}

type subscriptionTMDBTVDetailResponse struct {
	Seasons []struct {
		SeasonNumber int `json:"season_number"`
	} `json:"seasons"`
}

type subscriptionTMDBSeasonResponse struct {
	Episodes []struct {
		EpisodeNumber int    `json:"episode_number"`
		AirDate       string `json:"air_date"`
	} `json:"episodes"`
}

type subscriptionEpisodeInventory map[int]map[int]struct{}

type wholeShowIgnoredEpisodeSet map[string]struct{}

type wholeShowEpisodeRef struct {
	Season  int
	Episode int
}

type seriesTMDBLookupCacheEntry struct {
	value     string
	expiresAt time.Time
}

const subscriptionSeriesTMDBLookupCacheTTL = 5 * time.Minute

var subscriptionSeriesTMDBLookupNow = func() time.Time {
	return time.Now().UTC()
}

var subscriptionSeriesTMDBLookupCache = struct {
	mu      sync.RWMutex
	entries map[string]seriesTMDBLookupCacheEntry
}{
	entries: make(map[string]seriesTMDBLookupCacheEntry),
}

var newSubscriptionEmbyLookup = func() subscriptionEmbyLookup {
	return embyint.GetSharedService()
}

var activeSubscriptionStatuses = []models.SubscriptionStatus{
	models.SubscriptionPending,
	models.SubscriptionApproved,
	models.SubscriptionIngested,
}

var loadSubscriptionForAdminNotificationSync = func(subscriptionID string) (models.Subscription, error) {
	var subscription models.Subscription
	err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error
	return subscription, err
}

// fetchSubscriptionByID 用于手工补偿 / 管理员入库收口等需要按 id 查询单条订阅的入口。
// 拆成包级变量既能让 prepareManualSubscription / MarkSubscriptionIngestedAsAdmin 复用，
// 也方便单元测试替换为纯函数实现，不依赖真实 DB 连接。
var fetchSubscriptionByID = func(subscriptionID string) (models.Subscription, error) {
	var subscription models.Subscription
	err := db.DB.Where("id = ?", strings.TrimSpace(subscriptionID)).First(&subscription).Error
	return subscription, err
}

// persistSubscriptionMpError 写回订阅的 MoviePilot 自动订阅错误字段。
// 拆成包级变量，让手动补偿 service 测试能断言是否触发写库副作用。
var persistSubscriptionMpError = func(subscriptionID string, mpError *string) error {
	return db.DB.Model(&models.Subscription{}).
		Where("id = ? AND status = ?", subscriptionID, models.SubscriptionApproved).
		Update("mp_error", mpError).Error
}

var runAdminNotificationSync = func(s *SubscriptionService, subscription models.Subscription) {
	s.syncAdminNotifications(subscription)
}

const (
	subscriptionAdminNotificationSent       = "sent"
	subscriptionAdminNotificationSendFailed = "send_failed"
	subscriptionAdminNotificationEditFailed = "edit_failed"
	subscriptionAdminNotificationDeleted    = "deleted"
)

// NewSubscriptionService 创建订阅服务
func NewSubscriptionService() *SubscriptionService {
	return &SubscriptionService{
		moviepilot: moviepilotint.NewMoviePilotClient(),
		notifier:   notifierint.GetSharedBotNotifier(),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func normalizeSubscriptionSeason(mediaType models.MediaType, season int) (int, error) {
	if season < 0 {
		return 0, ErrSubscriptionInvalidSeason
	}
	if mediaType == models.MediaMovie {
		return 0, nil
	}
	return season, nil
}

func hasActiveSubscription(mediaType models.MediaType, tmdbID string, season int) (bool, error) {
	var count int64
	if err := db.DB.Model(&models.Subscription{}).
		Where("type = ? AND \"tmdb_id\" = ? AND season = ? AND status IN ?", mediaType, strings.TrimSpace(tmdbID), season, activeSubscriptionStatuses).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func isSubscriptionUniqueConflict(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505"
}

func buildConfirmationRequiredResult(checkResult *CheckExistingResponse) *CreateSubscriptionResult {
	return &CreateSubscriptionResult{
		Success:              false,
		ConfirmationRequired: true,
		DetectionFailed:      checkResult.DetectionFailed,
		ExistingSummary:      checkResult.ExistingSummary,
	}
}

func (s *SubscriptionService) requireExistingConfirmation(userID string, mediaType models.MediaType, tmdbID string, season int, action string) (*CreateSubscriptionResult, error) {
	checkResult, err := s.CheckExisting(CheckExistingRequest{
		Type:   mediaType,
		TmdbID: tmdbID,
		Season: season,
	})
	if err != nil {
		return nil, err
	}
	if checkResult.ExistsInLibrary || checkResult.DetectionFailed {
		log.Printf("[Subscription] %s 前需要二次确认 userId=%s type=%s tmdbId=%s season=%d exists=%t detectionFailed=%t", action, userID, mediaType, tmdbID, season, checkResult.ExistsInLibrary, checkResult.DetectionFailed)
		return buildConfirmationRequiredResult(checkResult), nil
	}
	return nil, nil
}

// CreateSubscription 兼容旧调用方的创建入口。
func (s *SubscriptionService) CreateSubscription(userID string, req CreateSubscriptionRequest) error {
	result, err := s.CreateSubscriptionWithResult(userID, req)
	if err != nil {
		return err
	}
	if result != nil && result.ConfirmationRequired {
		return errors.New("库内已存在相关资源，请确认后继续提交")
	}
	return nil
}

// CreateSubscriptionWithResult 创建订阅并返回结构化结果。
func (s *SubscriptionService) CreateSubscriptionWithResult(userID string, req CreateSubscriptionRequest) (*CreateSubscriptionResult, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.TmdbID) == "" {
		return nil, errors.New("影视名称和 TMDB ID 为必填项")
	}

	season, err := normalizeSubscriptionSeason(req.Type, req.Season)
	if err != nil {
		return nil, err
	}
	req.TmdbID = strings.TrimSpace(req.TmdbID)
	req.Name = strings.TrimSpace(req.Name)

	// 不带 ConfirmExisting 的请求：先做"库内已存在"二次确认（不写状态，外部 Emby 调用）。
	if !req.ConfirmExisting {
		confirmation, err := s.requireExistingConfirmation(userID, req.Type, req.TmdbID, season, "创建")
		if err != nil {
			return nil, err
		}
		if confirmation != nil {
			return confirmation, nil
		}
	}

	subscription := &models.Subscription{
		UserID:     userID,
		Type:       req.Type,
		Name:       req.Name,
		TmdbID:     req.TmdbID,
		Season:     season,
		PosterPath: req.PosterPath,
		Note:       ptrToString(req.Note),
		Status:     models.SubscriptionPending,
	}

	created, existingID, err := s.insertSubscriptionWithLock(req.Type, req.TmdbID, season, subscription)
	if err != nil {
		return nil, err
	}
	if !created {
		log.Printf("[Subscription] 创建命中已有活跃订阅，幂等返回 userId=%s existingSubscriptionId=%s type=%s tmdbId=%s season=%d",
			userID, existingID, req.Type, req.TmdbID, season)
		return &CreateSubscriptionResult{Success: true, SubscriptionID: existingID, AlreadyExists: true}, nil
	}

	log.Printf("[Subscription] 创建成功 userId=%s subscriptionId=%s type=%s tmdbId=%s season=%d", userID, subscription.ID, req.Type, req.TmdbID, season)
	async.SafeGo("subscription.notifyNew", func() {
		s.notifyNewSubscription(subscription.ID, userID, req, season)
	})

	return &CreateSubscriptionResult{Success: true, SubscriptionID: subscription.ID}, nil
}

// ResubmitSubscriptionWithResult 从已拒绝记录重新提交一条新的待审核订阅。
func (s *SubscriptionService) ResubmitSubscriptionWithResult(userID, subscriptionID string, req ResubmitSubscriptionRequest) (*CreateSubscriptionResult, error) {
	note := strings.TrimSpace(req.Note)
	if note == "" {
		return nil, ErrSubscriptionResubmitNote
	}

	var original models.Subscription
	if err := db.DB.Where("id = ? AND \"user_id\" = ?", subscriptionID, userID).First(&original).Error; err != nil {
		return nil, ErrSubscriptionNotFound
	}
	if original.Status != models.SubscriptionRejected {
		return nil, ErrSubscriptionNotFound
	}

	if !req.ConfirmExisting {
		confirmation, err := s.requireExistingConfirmation(userID, original.Type, original.TmdbID, original.Season, "重新提交")
		if err != nil {
			return nil, err
		}
		if confirmation != nil {
			return confirmation, nil
		}
	}

	retryFromID := original.ID
	subscription := &models.Subscription{
		UserID:      userID,
		Type:        original.Type,
		Name:        original.Name,
		TmdbID:      original.TmdbID,
		Season:      original.Season,
		PosterPath:  original.PosterPath,
		Note:        note,
		Status:      models.SubscriptionPending,
		RetryFromID: &retryFromID,
	}

	created, existingID, err := s.insertSubscriptionWithLock(original.Type, original.TmdbID, original.Season, subscription)
	if err != nil {
		return nil, err
	}
	if !created {
		log.Printf("[Subscription] 重新提交命中已有活跃订阅，幂等返回 userId=%s originalSubscriptionId=%s existingSubscriptionId=%s",
			userID, original.ID, existingID)
		return &CreateSubscriptionResult{Success: true, SubscriptionID: existingID, AlreadyExists: true}, nil
	}

	log.Printf("[Subscription] 重新提交成功 userId=%s originalSubscriptionId=%s subscriptionId=%s type=%s tmdbId=%s season=%d", userID, original.ID, subscription.ID, original.Type, original.TmdbID, original.Season)
	resubmitReq := CreateSubscriptionRequest{
		Type:       original.Type,
		Name:       original.Name,
		TmdbID:     original.TmdbID,
		Season:     original.Season,
		PosterPath: original.PosterPath,
		Note:       &note,
	}
	async.SafeGo("subscription.notifyResubmit", func() {
		s.notifyNewSubscription(subscription.ID, userID, resubmitReq, original.Season)
	})

	return &CreateSubscriptionResult{Success: true, SubscriptionID: subscription.ID}, nil
}

// insertSubscriptionWithLock 在 (type, tmdbId, season) 维度的 advisory lock 下完成"先查活跃→否则插入"的原子操作。
//
// 返回 (created, existingID, err)：
//   - created=true 表示本次插入了新行（subscription 已被 GORM 回填 ID）
//   - created=false 表示同 (type, tmdbId, season) 已有活跃订阅，existingID 为该订阅 ID（幂等返回）
//
// advisory lock 仅在事务内有效，事务提交 / 回滚后自动释放，能在多副本部署下序列化同一资源的并发写。
func (s *SubscriptionService) insertSubscriptionWithLock(mediaType models.MediaType, tmdbID string, season int, subscription *models.Subscription) (bool, string, error) {
	tmdbID = strings.TrimSpace(tmdbID)
	lockKey := fmt.Sprintf("subscription:%s:%s:%d", mediaType, tmdbID, season)

	tx := db.DB.Begin()
	if tx.Error != nil {
		return false, "", fmt.Errorf("创建订阅失败: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", lockKey).Error; err != nil {
		tx.Rollback()
		return false, "", fmt.Errorf("创建订阅失败: %w", err)
	}

	var existing models.Subscription
	err := tx.Where("type = ? AND \"tmdb_id\" = ? AND season = ? AND status IN ?",
		mediaType, tmdbID, season, activeSubscriptionStatuses).
		Order(`"created_at" ASC`).
		First(&existing).Error
	if err == nil {
		if commitErr := tx.Commit().Error; commitErr != nil {
			return false, "", fmt.Errorf("创建订阅失败: %w", commitErr)
		}
		return false, existing.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		tx.Rollback()
		return false, "", fmt.Errorf("创建订阅失败: %w", err)
	}

	if err := tx.Create(subscription).Error; err != nil {
		tx.Rollback()
		// 理论上 advisory lock 已经序列化，进到这里仍冲突极少见；走幂等回查保险。
		if isSubscriptionUniqueConflict(err) {
			var conflict models.Subscription
			if findErr := db.DB.Where("type = ? AND \"tmdb_id\" = ? AND season = ? AND status IN ?",
				mediaType, tmdbID, season, activeSubscriptionStatuses).
				Order(`"created_at" ASC`).
				First(&conflict).Error; findErr == nil {
				return false, conflict.ID, nil
			}
			return false, "", ErrSubscriptionDuplicated
		}
		return false, "", fmt.Errorf("创建订阅失败: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return false, "", fmt.Errorf("创建订阅失败: %w", err)
	}
	return true, subscription.ID, nil
}

func (s *SubscriptionService) notifyNewSubscription(subscriptionID, userID string, req CreateSubscriptionRequest, season int) {
	if s.notifier == nil || !s.notifier.IsConfigured() {
		return
	}

	var username string
	var user models.User
	if err := db.DB.Select("username").Where("id = ?", userID).First(&user).Error; err == nil {
		username = user.Username
	}

	payload := notifierint.SubscriptionNotification{
		ID:         subscriptionID,
		UserName:   username,
		Type:       string(req.Type),
		Name:       req.Name,
		TmdbID:     req.TmdbID,
		Season:     season,
		PosterPath: req.PosterPath,
		Note:       req.Note,
	}

	deliveries, err := s.notifier.NotifyNewSubscriptionWithDeliveries(payload)
	if err != nil {
		log.Printf("[Subscription] 管理员审批通知发送失败 subscriptionId=%s err=%v", subscriptionID, err)
		return
	}
	if err := s.persistAdminNotificationDeliveries(subscriptionID, deliveries); err != nil {
		log.Printf("[Subscription] 管理员审批通知投递记录写入失败 subscriptionId=%s count=%d err=%v", subscriptionID, len(deliveries), err)
		return
	}
	s.syncAdminNotificationsAfterDeliveryIfHandled(subscriptionID)
}

// persistAdminNotificationDeliveries 持久化 Bot 返回的管理员审批消息投递引用。
func (s *SubscriptionService) persistAdminNotificationDeliveries(subscriptionID string, deliveries []notifierint.SubscriptionAdminDelivery) error {
	if len(deliveries) == 0 {
		log.Printf("[Subscription] 管理员审批通知无投递结果 subscriptionId=%s", subscriptionID)
		return nil
	}

	records := make([]models.SubscriptionAdminNotification, 0, len(deliveries))
	for _, delivery := range deliveries {
		status := strings.TrimSpace(delivery.DeliveryStatus)
		if status == "" {
			status = subscriptionAdminNotificationSent
		}
		if delivery.AdminTelegramID <= 0 || delivery.ChatID == 0 {
			log.Printf("[Subscription] 跳过无效管理员通知投递 subscriptionId=%s adminTelegramId=%d chatId=%d status=%s",
				subscriptionID, delivery.AdminTelegramID, delivery.ChatID, status)
			continue
		}
		if delivery.MessageID == nil && status == subscriptionAdminNotificationSent {
			status = subscriptionAdminNotificationSendFailed
		}
		records = append(records, models.SubscriptionAdminNotification{
			SubscriptionID:  subscriptionID,
			AdminTelegramID: delivery.AdminTelegramID,
			ChatID:          delivery.ChatID,
			MessageID:       delivery.MessageID,
			HasPhoto:        delivery.HasPhoto,
			DeliveryStatus:  status,
			FailureReason:   delivery.FailureReason,
		})
	}
	if len(records) == 0 {
		return nil
	}

	return db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&records).Error
}

// shouldSyncAdminNotificationsAfterDelivery 判断晚到的管理员消息投递记录是否需要补偿同步。
func shouldSyncAdminNotificationsAfterDelivery(status models.SubscriptionStatus) bool {
	return status != models.SubscriptionPending
}

// syncAdminNotificationsAfterDeliveryIfHandled 在管理员消息引用晚于审批落库时补做最终状态同步。
func (s *SubscriptionService) syncAdminNotificationsAfterDeliveryIfHandled(subscriptionID string) {
	subscription, err := loadSubscriptionForAdminNotificationSync(subscriptionID)
	if err != nil {
		log.Printf("[Subscription] 管理员审批通知投递后查询订阅失败 subscriptionId=%s err=%v", subscriptionID, err)
		return
	}
	if !shouldSyncAdminNotificationsAfterDelivery(subscription.Status) {
		return
	}

	log.Printf("[Subscription] 管理员审批通知投递晚于审批结果，补偿同步 subscriptionId=%s status=%s", subscription.ID, subscription.Status)
	runAdminNotificationSync(s, subscription)
}

// GetUserSubscriptions 获取用户的订阅列表
func (s *SubscriptionService) GetUserSubscriptions(userID string) ([]models.Subscription, error) {
	var subscriptions []models.Subscription
	err := db.DB.
		Where("\"user_id\" = ?", userID).
		Order("\"created_at\" DESC").
		Find(&subscriptions).Error
	if err != nil {
		return nil, fmt.Errorf("查询订阅列表失败: %w", err)
	}
	return subscriptions, nil
}

// GetUserSubscriptionsPaginated 用户订阅分页查询
func (s *SubscriptionService) GetUserSubscriptionsPaginated(userID string, status *models.SubscriptionStatus, page, pageSize int) (*GetAllSubscriptionsResponse, error) {
	offset := (page - 1) * pageSize
	query := db.DB.Model(&models.Subscription{}).Where("\"user_id\" = ?", userID)
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("查询订阅总数失败: %w", err)
	}

	var subscriptions []models.Subscription
	if err := query.Order("\"created_at\" DESC").Offset(offset).Limit(pageSize).Find(&subscriptions).Error; err != nil {
		return nil, fmt.Errorf("查询订阅列表失败: %w", err)
	}

	result := make([]SubscriptionWithUser, len(subscriptions))
	for i, sub := range subscriptions {
		result[i] = SubscriptionWithUser{Subscription: sub}
	}

	return &GetAllSubscriptionsResponse{
		Data:  result,
		Total: total,
	}, nil
}

// DeleteSubscription 删除订阅（仅允许删除 PENDING 状态）
func (s *SubscriptionService) DeleteSubscription(subscriptionID, userID string) error {
	var subscription models.Subscription
	if err := db.DB.Where("id = ? AND \"user_id\" = ?", subscriptionID, userID).First(&subscription).Error; err != nil {
		return ErrSubscriptionNotFound
	}
	if subscription.Status != models.SubscriptionPending {
		return ErrSubscriptionNotFound
	}
	if err := db.DB.Delete(&subscription).Error; err != nil {
		return fmt.Errorf("删除订阅失败: %w", err)
	}
	return nil
}

// DeleteSubscriptionAsAdmin 管理员删除订阅（不限制状态和所有权）
func (s *SubscriptionService) DeleteSubscriptionAsAdmin(subscriptionID string) error {
	var subscription models.Subscription
	if err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return ErrSubscriptionNotFound
	}
	if err := db.DB.Delete(&subscription).Error; err != nil {
		return fmt.Errorf("删除订阅失败: %w", err)
	}
	return nil
}

// GetAllSubscriptions 管理员获取所有订阅（分页）
func (s *SubscriptionService) GetAllSubscriptions(status *models.SubscriptionStatus, page, pageSize int) (*GetAllSubscriptionsResponse, error) {
	offset := (page - 1) * pageSize
	query := db.DB.Model(&models.Subscription{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("查询订阅总数失败: %w", err)
	}

	var subscriptions []models.Subscription
	if err := query.Order("\"created_at\" DESC").Offset(offset).Limit(pageSize).Find(&subscriptions).Error; err != nil {
		return nil, fmt.Errorf("查询订阅列表失败: %w", err)
	}

	userIDs := make([]string, len(subscriptions))
	for i, sub := range subscriptions {
		userIDs[i] = sub.UserID
	}

	var users []models.User
	if len(userIDs) > 0 {
		if err := db.DB.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, fmt.Errorf("查询用户信息失败: %w", err)
		}
	}

	userMap := make(map[string]*models.User, len(users))
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}

	result := make([]SubscriptionWithUser, len(subscriptions))
	for i, sub := range subscriptions {
		result[i].Subscription = sub
		if user, ok := userMap[sub.UserID]; ok {
			result[i].User.Username = user.Username
			result[i].User.Email = user.Email
		}
	}

	return &GetAllSubscriptionsResponse{
		Data:  result,
		Total: total,
	}, nil
}

// ApproveSubscription 批准订阅。原子状态转移：UPDATE WHERE status='PENDING'，
// RowsAffected=0 视为状态冲突（可能被并发审批 / webhook 抢先收口）→ ErrSubscriptionStateConflict。
//
// MoviePilot 调用从同步路径剥离到 commit 后异步执行：
//   - 调用成功：清空 mpError
//   - 调用失败：写 mpError，状态保持 APPROVED（管理员可通过 redispatch 重试）
func (s *SubscriptionService) ApproveSubscription(subscriptionID string) error {
	var subscription models.Subscription
	if err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return ErrSubscriptionNotFound
	}
	if subscription.Status != models.SubscriptionPending {
		return ErrSubscriptionHandled
	}

	now := time.Now().UTC()
	result := db.DB.Model(&models.Subscription{}).
		Where("id = ? AND status = ?", subscription.ID, models.SubscriptionPending).
		Updates(map[string]interface{}{
			"status":        models.SubscriptionApproved,
			"reviewed_at":   now,
			"reject_reason": nil,
			"mp_error":      nil,
		})
	if result.Error != nil {
		return fmt.Errorf("更新订阅状态失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		log.Printf("[Subscription] 审批 race 冲突 subscriptionId=%s currentStatus=%s", subscription.ID, subscription.Status)
		return ErrSubscriptionStateConflict
	}

	subscription.Status = models.SubscriptionApproved
	subscription.ReviewedAt = &now
	subscription.RejectReason = nil
	subscription.MpError = nil

	log.Printf("[Subscription] 审批通过 subscriptionId=%s userId=%s type=%s tmdbId=%s season=%d",
		subscription.ID, subscription.UserID, subscription.Type, subscription.TmdbID, subscription.Season)

	subscriptionID = subscription.ID
	subscriptionType := subscription.Type
	subscriptionName := subscription.Name
	tmdbID := subscription.TmdbID
	season := subscription.Season
	async.SafeGo("subscription.dispatchMoviePilot", func() {
		s.dispatchMoviePilotAsync(subscriptionID, subscriptionType, subscriptionName, tmdbID, season)
	})
	async.SafeGo("subscription.notifyApproved", func() { s.notifyApproved(subscription) })
	async.SafeGo("subscription.syncAdminApproved", func() { s.syncAdminNotifications(subscription) })
	return nil
}

// dispatchMoviePilotAsync 在事务外异步调用 MoviePilot；失败时写 mpError，状态保持 APPROVED。
func (s *SubscriptionService) dispatchMoviePilotAsync(subscriptionID string, mediaType models.MediaType, name, tmdbID string, season int) {
	if !s.moviepilot.IsConfigured() {
		errMsg := "MoviePilot 未配置"
		log.Printf("[Subscription] 跳过 MoviePilot：未配置 subscriptionId=%s type=%s tmdbId=%s season=%d",
			subscriptionID, mediaType, tmdbID, season)
		s.persistMpError(subscriptionID, &errMsg)
		return
	}

	mpType := "movie"
	if mediaType == models.MediaTV {
		mpType = "tv"
	}
	if err := s.moviepilot.CreateSubscription(moviepilotint.SubscribeRequest{
		Type:   mpType,
		Name:   name,
		TmdbID: tmdbID,
		Season: season,
	}); err != nil {
		errMsg := err.Error()
		log.Printf("[Subscription] MoviePilot 调用失败 subscriptionId=%s type=%s tmdbId=%s season=%d err=%v",
			subscriptionID, mediaType, tmdbID, season, err)
		s.persistMpError(subscriptionID, &errMsg)
		return
	}

	s.persistMpError(subscriptionID, nil)
}

func (s *SubscriptionService) persistMpError(subscriptionID string, mpError *string) {
	if err := persistSubscriptionMpError(subscriptionID, mpError); err != nil {
		log.Printf("[Subscription] 写回 mpError 失败 subscriptionId=%s mpError=%v err=%v", subscriptionID, mpError, err)
	}
}

// RejectSubscription 拒绝订阅。原子状态转移：UPDATE WHERE status='PENDING'，
// RowsAffected=0 视为状态冲突（与 ApproveSubscription 同语义）→ ErrSubscriptionStateConflict。
func (s *SubscriptionService) RejectSubscription(subscriptionID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrSubscriptionRejectReason
	}

	var subscription models.Subscription
	if err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return ErrSubscriptionNotFound
	}
	if subscription.Status != models.SubscriptionPending {
		return ErrSubscriptionHandled
	}

	now := time.Now().UTC()
	result := db.DB.Model(&models.Subscription{}).
		Where("id = ? AND status = ?", subscription.ID, models.SubscriptionPending).
		Updates(map[string]interface{}{
			"status":        models.SubscriptionRejected,
			"reviewed_at":   now,
			"reject_reason": reason,
		})
	if result.Error != nil {
		return fmt.Errorf("更新订阅状态失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		log.Printf("[Subscription] 拒绝 race 冲突 subscriptionId=%s currentStatus=%s", subscription.ID, subscription.Status)
		return ErrSubscriptionStateConflict
	}

	subscription.Status = models.SubscriptionRejected
	subscription.ReviewedAt = &now
	subscription.RejectReason = &reason

	log.Printf("[Subscription] 审批拒绝 subscriptionId=%s userId=%s type=%s tmdbId=%s season=%d reason=%q",
		subscription.ID, subscription.UserID, subscription.Type, subscription.TmdbID, subscription.Season, reason)
	async.SafeGo("subscription.notifyRejected", func() { s.notifyRejected(subscription) })
	async.SafeGo("subscription.syncAdminRejected", func() { s.syncAdminNotifications(subscription) })
	return nil
}

// RedispatchSubscription 管理员手动重试 MoviePilot 调用：
// 仅在 status='APPROVED' 且 mpError 非空时允许；状态保持 APPROVED；mpError 由本次结果覆盖。
func (s *SubscriptionService) RedispatchSubscription(subscriptionID string) error {
	var subscription models.Subscription
	if err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return ErrSubscriptionNotFound
	}
	if subscription.Status != models.SubscriptionApproved || subscription.MpError == nil || strings.TrimSpace(*subscription.MpError) == "" {
		return ErrSubscriptionRedispatchSafe
	}

	if !s.moviepilot.IsConfigured() {
		errMsg := "MoviePilot 未配置"
		s.persistMpError(subscription.ID, &errMsg)
		log.Printf("[Subscription] redispatch 失败：MoviePilot 未配置 subscriptionId=%s", subscription.ID)
		return ErrSubscriptionRedispatchSafe
	}

	mpType := "movie"
	if subscription.Type == models.MediaTV {
		mpType = "tv"
	}
	if err := s.moviepilot.CreateSubscription(moviepilotint.SubscribeRequest{
		Type:   mpType,
		Name:   subscription.Name,
		TmdbID: subscription.TmdbID,
		Season: subscription.Season,
	}); err != nil {
		errMsg := err.Error()
		s.persistMpError(subscription.ID, &errMsg)
		log.Printf("[Subscription] redispatch MoviePilot 仍失败 subscriptionId=%s tmdbId=%s season=%d err=%v",
			subscription.ID, subscription.TmdbID, subscription.Season, err)
		return fmt.Errorf("MoviePilot 调用失败: %w", err)
	}

	s.persistMpError(subscription.ID, nil)
	log.Printf("[Subscription] redispatch 成功 subscriptionId=%s tmdbId=%s season=%d", subscription.ID, subscription.TmdbID, subscription.Season)
	return nil
}

// ManualSearchSubscription 按订阅 TMDB ID 搜索 MoviePilot 手动补偿候选。
func (s *SubscriptionService) ManualSearchSubscription(subscriptionID string, req ManualSearchRequest) (*ManualSearchResult, error) {
	subscription, season, err := s.prepareManualSubscription(subscriptionID, req.Season, true)
	if err != nil {
		return nil, err
	}

	mediaType := "movie"
	if subscription.Type == models.MediaTV {
		mediaType = "tv"
	}

	searchResp, err := s.moviepilot.SearchMediaCandidates(moviepilotint.SearchMediaRequest{
		TmdbID:    subscription.TmdbID,
		MediaType: mediaType,
		Season:    season,
	})
	if err != nil {
		log.Printf("[Subscription] 手动补下载搜索失败 subscriptionId=%s tmdbId=%s season=%d err=%v", subscription.ID, subscription.TmdbID, season, err)
		return nil, fmt.Errorf("MoviePilot 手动搜索失败: %w", upstream.SafeUpstreamError(err, "moviepilot"))
	}
	if searchResp == nil {
		searchResp = &moviepilotint.SearchMediaResponse{Candidates: []moviepilotint.SearchCandidate{}}
	}

	log.Printf("[Subscription] 手动补下载搜索完成 subscriptionId=%s tmdbId=%s season=%d candidates=%d",
		subscription.ID, subscription.TmdbID, season, len(searchResp.Candidates))
	return &ManualSearchResult{
		Subscription: subscription,
		Source:       "MoviePilot",
		Query:        searchResp.Query,
		MatchMode:    searchResp.MatchMode,
		Candidates:   searchResp.Candidates,
	}, nil
}

// ManualDispatchSubscription 将管理员选定的候选资源直接下发给 MoviePilot 下载。
func (s *SubscriptionService) ManualDispatchSubscription(subscriptionID string, req ManualDispatchRequest) (*ManualDispatchResult, error) {
	subscription, season, err := s.prepareManualSubscription(subscriptionID, req.Season, true)
	if err != nil {
		return nil, err
	}

	payload := req.CandidatePayload
	if len(payload) == 0 && req.Candidate != nil {
		payload = req.Candidate.Payload
	}
	if len(payload) == 0 {
		return nil, ErrSubscriptionManualCandidate
	}

	dispatchResp, err := s.moviepilot.DispatchDownloadCandidate(moviepilotint.DownloadCandidateRequest{
		CandidatePayload: payload,
		TmdbID:           subscription.TmdbID,
		Season:           season,
	})
	if err != nil {
		// 手动下发失败不写 mpError：mpError 只反映"自动 MoviePilot 订阅创建"链路，
		// 而 redispatch 按钮可见条件是 mpError 非空、且它干的是"重试订阅创建"（不同链路）。
		// 混用会让管理员用 redispatch 重试一个本应走手动补偿的失败。失败只靠 API 同步报错回显。
		//
		// 业务拒绝（重复添加等）与基础设施错误需要分别处理：
		//   - ErrMoviePilotBusinessRejected：上游服务正常，仅业务规则不满足。错误已脱敏，
		//     直接透传业务原因，避免被 SafeUpstreamError 截断成 "upstream moviepilot unavailable"。
		//   - 其他：基础设施错误，仍用 SafeUpstreamError 脱敏后包装，避免回显 URL/凭证。
		wrappedErr := upstream.SafeUpstreamError(err, "moviepilot")
		if errors.Is(err, moviepilotint.ErrMoviePilotBusinessRejected) {
			wrappedErr = err
		}
		log.Printf("[Subscription] 手动补下载下发失败 subscriptionId=%s tmdbId=%s payloadKeys=%d businessRejected=%t err=%v",
			subscription.ID, subscription.TmdbID, len(payload), errors.Is(err, moviepilotint.ErrMoviePilotBusinessRejected), wrappedErr)
		return nil, fmt.Errorf("MoviePilot 手动下发失败: %w", wrappedErr)
	}

	s.persistMpError(subscription.ID, nil)
	subscription.MpError = nil
	message := ""
	if dispatchResp != nil {
		message = strings.TrimSpace(dispatchResp.Message)
	}
	log.Printf("[Subscription] 手动补下载下发完成 subscriptionId=%s tmdbId=%s payloadKeys=%d message=%q",
		subscription.ID, subscription.TmdbID, len(payload), message)
	return &ManualDispatchResult{
		Subscription: subscription,
		Accepted:     true,
		Message:      message,
	}, nil
}

func (s *SubscriptionService) prepareManualSubscription(subscriptionID string, requestedSeason *int, requireTVSeason bool) (models.Subscription, int, error) {
	subscription, err := fetchSubscriptionByID(subscriptionID)
	if err != nil {
		// 区分"记录不存在（404）"与"DB 故障 / 超时（500）"：前者是合法的用户可见错误，
		// 后者走 default 分支避免把基础设施故障误判成 NotFound。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return subscription, 0, ErrSubscriptionNotFound
		}
		return subscription, 0, fmt.Errorf("查询订阅失败: %w", err)
	}
	if subscription.Status != models.SubscriptionApproved {
		return subscription, 0, ErrSubscriptionNotApproved
	}
	// 提前校验 tmdbId 数字格式，避免非数字值一路放行到 MoviePilot 客户端 Atoi 失败，
	// 再被 upstream.SafeUpstreamError 包装成 500 上游故障。这里返回 400 业务错误。
	if _, err := strconv.Atoi(strings.TrimSpace(subscription.TmdbID)); err != nil {
		return subscription, 0, ErrSubscriptionInvalidTMDBID
	}

	season := 0
	if subscription.Type == models.MediaTV {
		season = subscription.Season
		if season <= 0 && requireTVSeason {
			if requestedSeason == nil || *requestedSeason <= 0 {
				return subscription, 0, ErrSubscriptionManualSeason
			}
			season = *requestedSeason
		}
	}
	return subscription, season, nil
}

// MarkSubscriptionIngestedAsAdmin 允许管理员在校验 Emby 已存在资源后手动收口为已入库。
func (s *SubscriptionService) MarkSubscriptionIngestedAsAdmin(subscriptionID string) error {
	subscription, err := fetchSubscriptionByID(subscriptionID)
	if err != nil {
		// 区分"记录不存在（404）"与"DB 故障 / 超时（500）"，避免基础设施故障被误判成 NotFound。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSubscriptionNotFound
		}
		return fmt.Errorf("查询订阅失败: %w", err)
	}
	if subscription.Status != models.SubscriptionApproved {
		return ErrSubscriptionNotApproved
	}

	embyService := embyint.GetSharedService()
	if !embyService.IsConfigured() {
		return errors.New("Emby 未配置，无法校验入库状态")
	}

	existing, err := checkExistingInLibrary(embyService, CheckExistingRequest{
		Type:   subscription.Type,
		TmdbID: subscription.TmdbID,
		Season: subscription.Season,
	})
	if err != nil {
		return fmt.Errorf("校验 Emby 入库状态失败: %w", err)
	}
	if existing == nil || !existing.ExistsInLibrary {
		log.Printf("[Subscription] 管理员校验入库未命中 subscriptionId=%s userId=%s type=%s tmdbId=%s season=%d", subscription.ID, subscription.UserID, subscription.Type, subscription.TmdbID, subscription.Season)
		return ErrSubscriptionNotInLibrary
	}

	now := time.Now().UTC()
	result := db.DB.Model(&models.Subscription{}).
		Where("id = ? AND status = ?", subscription.ID, models.SubscriptionApproved).
		Updates(map[string]interface{}{
			"status":      models.SubscriptionIngested,
			"ingested_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("更新订阅入库状态失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrSubscriptionNotApproved
	}

	subscription.Status = models.SubscriptionIngested
	subscription.IngestedAt = &now

	log.Printf("[Subscription] 管理员校验后标记已入库 subscriptionId=%s userId=%s type=%s tmdbId=%s season=%d", subscription.ID, subscription.UserID, subscription.Type, subscription.TmdbID, subscription.Season)
	async.SafeGo("subscription.notifyIngested", func() { s.notifyIngested(subscription) })
	return nil
}

// MarkSubscriptionsIngestedByWebhook 将命中的 APPROVED 订阅按整剧 / 单季拆分回写：
//   - 单季订阅 (subscriptions.season = N)：webhook 命中即收口为 INGESTED（与原行为一致，但只匹配 season=N）
//   - 整剧订阅 (subscriptions.season = 0)：仅更新 ingestProgress 文本，不收口为 INGESTED
//     管理员需通过 MarkSubscriptionIngestedAsAdmin 显式确认整剧入库完成
//
// 拆分动机：原 WHERE `season = 0 OR season = ?` 把任意单集 webhook 同时收口为整剧 INGESTED，
// 让用户和管理员误以为整剧入库完成，掩盖大量缺集。
func (s *SubscriptionService) MarkSubscriptionsIngestedByWebhook(ctx context.Context, payload SubscriptionIngestWebhookPayload) (int64, error) {
	itemType := strings.ToLower(strings.TrimSpace(payload.ItemType))
	matchTMDBIDs := s.resolveWebhookMatchTMDBIDs(itemType, payload)
	if len(matchTMDBIDs) == 0 {
		log.Printf("[Subscription] Webhook 跳过入库回写：缺少可匹配的 tmdbId itemType=%s itemName=%q seriesId=%s", itemType, strings.TrimSpace(payload.ItemName), strings.TrimSpace(payload.SeriesID))
		return 0, nil
	}

	switch itemType {
	case "movie":
		return s.markMovieSubscriptionsIngested(ctx, matchTMDBIDs, payload)
	case "episode":
		if payload.Season <= 0 || payload.Episode <= 0 {
			log.Printf("[Subscription] Webhook 跳过入库回写：季集号无效 tmdbIds=%v season=%d episode=%d", matchTMDBIDs, payload.Season, payload.Episode)
			return 0, nil
		}
		return s.markEpisodeSubscriptionsIngested(ctx, matchTMDBIDs, payload)
	default:
		return 0, nil
	}
}

func (s *SubscriptionService) markMovieSubscriptionsIngested(ctx context.Context, matchTMDBIDs []string, payload SubscriptionIngestWebhookPayload) (int64, error) {
	var subscriptions []models.Subscription
	if err := db.DB.WithContext(ctx).
		Where("status = ? AND \"tmdb_id\" IN ? AND type = ? AND season = 0",
			models.SubscriptionApproved, matchTMDBIDs, models.MediaMovie).
		Find(&subscriptions).Error; err != nil {
		return 0, fmt.Errorf("查询待入库订阅失败: %w", err)
	}
	return s.collectIngestUpdates(ctx, subscriptions, payload)
}

func (s *SubscriptionService) markEpisodeSubscriptionsIngested(ctx context.Context, matchTMDBIDs []string, payload SubscriptionIngestWebhookPayload) (int64, error) {
	// 单季订阅：season=N 命中即 INGEST。
	var seasonSubs []models.Subscription
	if err := db.DB.WithContext(ctx).
		Where("status = ? AND \"tmdb_id\" IN ? AND type = ? AND season = ?",
			models.SubscriptionApproved, matchTMDBIDs, models.MediaTV, payload.Season).
		Find(&seasonSubs).Error; err != nil {
		return 0, fmt.Errorf("查询单季待入库订阅失败: %w", err)
	}
	updated, err := s.collectIngestUpdates(ctx, seasonSubs, payload)
	if err != nil {
		return updated, err
	}

	// 整剧订阅：season=0，按 "当前在 Emby 库内的已完成集数 Y / 已播出总集数 X" 推进 ingestProgress；Y >= X 时自动 INGEST。
	var wholeShowSubs []models.Subscription
	if err := db.DB.WithContext(ctx).
		Where("status = ? AND \"tmdb_id\" IN ? AND type = ? AND season = 0",
			models.SubscriptionApproved, matchTMDBIDs, models.MediaTV).
		Find(&wholeShowSubs).Error; err != nil {
		return updated, fmt.Errorf("查询整剧待入库订阅失败: %w", err)
	}
	now := time.Now().UTC()
	for _, sub := range wholeShowSubs {
		stats, statsErr := s.computeWholeShowProgress(ctx, sub.TmdbID, payload.SeriesID, now)
		if statsErr != nil {
			log.Printf("[Subscription] 整剧进度统计失败 subscriptionId=%s tmdbId=%s err=%v", sub.ID, sub.TmdbID, statsErr)
		}

		if stats != nil && stats.Total > 0 && stats.Done >= stats.Total {
			progress := fmt.Sprintf("%d/%d", stats.Done, stats.Total)
			result := db.DB.WithContext(ctx).Model(&models.Subscription{}).
				Where("id = ? AND status = ?", sub.ID, models.SubscriptionApproved).
				Updates(map[string]interface{}{
					"status":          models.SubscriptionIngested,
					"ingested_at":     now,
					"ingest_progress": progress,
				})
			if result.Error != nil {
				log.Printf("[Subscription] 整剧自动收口失败 subscriptionId=%s tmdbId=%s err=%v", sub.ID, sub.TmdbID, result.Error)
				continue
			}
			if result.RowsAffected == 0 {
				continue
			}
			sub.Status = models.SubscriptionIngested
			sub.IngestedAt = &now
			sub.IngestProgress = &progress
			updated += result.RowsAffected
			log.Printf("[Subscription] 整剧已播出集全部入库，自动收口为 INGESTED subscriptionId=%s userId=%s tmdbId=%s progress=%s",
				sub.ID, sub.UserID, sub.TmdbID, progress)
			subSnapshot := sub
			async.SafeGo("subscription.notifyIngested", func() { s.notifyIngested(subSnapshot) })
			continue
		}

		var progress string
		if stats != nil && stats.Total > 0 {
			progress = fmt.Sprintf("%d/%d", stats.Done, stats.Total)
		} else {
			// TMDB / Emby 任一侧暂不可用时，不冒险自动收口；fallback 记最近一集方便前端展示。
			progress = fmt.Sprintf("S%02dE%02d", payload.Season, payload.Episode)
		}

		if err := db.DB.WithContext(ctx).Model(&models.Subscription{}).
			Where("id = ? AND status = ?", sub.ID, models.SubscriptionApproved).
			Update("ingest_progress", progress).Error; err != nil {
			log.Printf("[Subscription] 整剧 ingestProgress 更新失败 subscriptionId=%s tmdbId=%s err=%v", sub.ID, sub.TmdbID, err)
			continue
		}
		log.Printf("[Subscription] 整剧 webhook 进度更新 subscriptionId=%s userId=%s tmdbId=%s progress=%s",
			sub.ID, sub.UserID, sub.TmdbID, progress)
	}

	return updated, nil
}

// wholeShowProgressStats 描述整剧订阅的入库进度统计。
type wholeShowProgressStats struct {
	Total int // X：已播出（airDate <= now，且未 IGNORED）的集数总和
	Done  int // Y：这些集里当前在 Emby 库中实际存在的集数
}

// computeWholeShowProgress 用 TMDB 已播出剧集清单 + Emby 实际库存 + IGNORED 集排除集，
// 计算整剧订阅当前的 (Y, X)。
//
// 为什么不能再用 media_gaps 直接反查：
//   - media_gaps 只记录"曾经缺过"的集，不覆盖一开始就已在库中的集
//   - 直接用它统计会把"所有缺过的集都补齐了"误判成"整剧已全部入库"
//
// 设计：
//   - X：TMDB 中 airDate <= 今天，且未被管理员 IGNORE 的剧集总数
//   - Y：上述剧集中，当前在 Emby 库内实际存在的集数
//   - TMDB / Emby 任一侧缺失时返回 nil，让调用方走最近一集 fallback，而不是错误自动收口
func (s *SubscriptionService) computeWholeShowProgress(ctx context.Context, tmdbID, seriesID string, now time.Time) (*wholeShowProgressStats, error) {
	tmdbID = strings.TrimSpace(tmdbID)
	if tmdbID == "" {
		return nil, nil
	}

	today := subscriptionCalendarDateInLocation(now, configpkg.LoadConfiguredTimezone())
	requiredEpisodes, err := s.loadWholeShowAiredEpisodes(ctx, tmdbID, today)
	if err != nil {
		return nil, err
	}
	if len(requiredEpisodes) == 0 {
		return &wholeShowProgressStats{}, nil
	}

	ignoredEpisodes, err := loadWholeShowIgnoredEpisodes(ctx, tmdbID, today)
	if err != nil {
		return nil, err
	}

	resolvedSeriesID, err := resolveWholeShowSeriesID(tmdbID, seriesID)
	if err != nil {
		return nil, err
	}
	if resolvedSeriesID == "" {
		return nil, nil
	}

	inventory, err := loadWholeShowEpisodeInventory(resolvedSeriesID)
	if err != nil {
		return nil, err
	}

	stats := buildWholeShowProgressStats(requiredEpisodes, inventory, ignoredEpisodes)
	return &stats, nil
}

func (s *SubscriptionService) loadWholeShowAiredEpisodes(ctx context.Context, tmdbID string, today time.Time) ([]wholeShowEpisodeRef, error) {
	if strings.TrimSpace(loadSubscriptionTMDBAPIKey()) == "" {
		return nil, nil
	}

	detail, err := s.fetchWholeShowTVDetail(ctx, tmdbID)
	if err != nil {
		return nil, fmt.Errorf("查询整剧 TMDB 详情失败: %w", err)
	}

	episodes := make([]wholeShowEpisodeRef, 0, 32)
	for _, seasonMeta := range detail.Seasons {
		seasonNumber := seasonMeta.SeasonNumber
		if seasonNumber <= 0 {
			continue
		}

		seasonDetail, err := s.fetchWholeShowSeasonDetail(ctx, tmdbID, seasonNumber)
		if err != nil {
			return nil, fmt.Errorf("查询整剧 TMDB 季详情失败: %w", err)
		}

		for _, episodeMeta := range seasonDetail.Episodes {
			if episodeMeta.EpisodeNumber <= 0 {
				continue
			}
			airDate, ok := parseSubscriptionTMDBAirDate(episodeMeta.AirDate)
			if !ok || airDate.After(today) {
				continue
			}
			episodes = append(episodes, wholeShowEpisodeRef{
				Season:  seasonNumber,
				Episode: episodeMeta.EpisodeNumber,
			})
		}
	}

	return episodes, nil
}

func loadWholeShowIgnoredEpisodes(ctx context.Context, tmdbID string, today time.Time) (wholeShowIgnoredEpisodeSet, error) {
	var ignored []models.MediaGap
	if err := db.DB.WithContext(ctx).
		Select("season", "episode", "\"ignore_reason\"").
		Where(`"tmdb_id" = ? AND "air_date" <= ? AND status = ?`, strings.TrimSpace(tmdbID), today, models.MediaGapStatusIgnored).
		Find(&ignored).Error; err != nil {
		return nil, fmt.Errorf("查询整剧已忽略剧集失败: %w", err)
	}

	result := make(wholeShowIgnoredEpisodeSet, len(ignored))
	for _, gap := range ignored {
		if strings.TrimSpace(gap.IgnoreReason) == "season not activated in library" {
			continue
		}
		result[buildWholeShowEpisodeKey(gap.Season, gap.Episode)] = struct{}{}
	}
	return result, nil
}

func resolveWholeShowSeriesID(tmdbID, seriesID string) (string, error) {
	trimmedSeriesID := strings.TrimSpace(seriesID)
	if trimmedSeriesID != "" {
		return trimmedSeriesID, nil
	}

	embyService := embyint.GetSharedService()
	if !embyService.IsConfigured() {
		return "", nil
	}

	item, err := findProviderMatchedItem(embyService, tmdbID, "Series")
	if err != nil {
		return "", fmt.Errorf("按 tmdbId 解析整剧 seriesId 失败: %w", err)
	}
	if item == nil {
		return "", nil
	}
	return strings.TrimSpace(item.ID), nil
}

func loadWholeShowEpisodeInventory(seriesID string) (subscriptionEpisodeInventory, error) {
	embyService := embyint.GetSharedService()
	if !embyService.IsConfigured() {
		return nil, nil
	}

	episodes, err := getSeriesEpisodes(embyService, seriesID)
	if err != nil {
		return nil, fmt.Errorf("查询整剧 Emby 库存失败: %w", err)
	}
	return buildSubscriptionEpisodeInventory(episodes), nil
}

func buildSubscriptionEpisodeInventory(items []embyExistingItem) subscriptionEpisodeInventory {
	inventory := make(subscriptionEpisodeInventory)
	for _, item := range items {
		if !hasPhysicalExistingItem(item) {
			continue
		}
		if item.ParentIndexNumber <= 0 || item.IndexNumber <= 0 {
			continue
		}
		if _, ok := inventory[item.ParentIndexNumber]; !ok {
			inventory[item.ParentIndexNumber] = make(map[int]struct{})
		}
		inventory[item.ParentIndexNumber][item.IndexNumber] = struct{}{}
	}
	return inventory
}

func (i subscriptionEpisodeInventory) has(season, episode int) bool {
	if i == nil {
		return false
	}
	episodes, ok := i[season]
	if !ok {
		return false
	}
	_, exists := episodes[episode]
	return exists
}

func buildWholeShowProgressStats(requiredEpisodes []wholeShowEpisodeRef, inventory subscriptionEpisodeInventory, ignoredEpisodes wholeShowIgnoredEpisodeSet) wholeShowProgressStats {
	stats := wholeShowProgressStats{}
	for _, episode := range requiredEpisodes {
		key := buildWholeShowEpisodeKey(episode.Season, episode.Episode)
		if _, ignored := ignoredEpisodes[key]; ignored {
			continue
		}

		stats.Total++
		if inventory.has(episode.Season, episode.Episode) {
			stats.Done++
		}
	}
	return stats
}

func buildWholeShowEpisodeKey(season, episode int) string {
	return fmt.Sprintf("%d:%d", season, episode)
}

func loadSubscriptionTMDBAPIKey() string {
	return strings.TrimSpace(configpkg.NewConfigService().GetString("TMDB_API_KEY"))
}

func (s *SubscriptionService) fetchWholeShowTVDetail(ctx context.Context, tmdbID string) (*subscriptionTMDBTVDetailResponse, error) {
	apiKey := loadSubscriptionTMDBAPIKey()
	if apiKey == "" {
		return nil, nil
	}

	endpoint := fmt.Sprintf("https://api.themoviedb.org/3/tv/%s?api_key=%s&language=zh-CN", url.PathEscape(strings.TrimSpace(tmdbID)), url.QueryEscape(apiKey))
	cacheKey := fmt.Sprintf("subscription:tv-detail:%s", strings.TrimSpace(tmdbID))
	var resp subscriptionTMDBTVDetailResponse
	if err := s.fetchSubscriptionTMDBJSON(ctx, cacheKey, endpoint, 24*time.Hour, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *SubscriptionService) fetchWholeShowSeasonDetail(ctx context.Context, tmdbID string, season int) (*subscriptionTMDBSeasonResponse, error) {
	apiKey := loadSubscriptionTMDBAPIKey()
	if apiKey == "" {
		return nil, nil
	}

	endpoint := fmt.Sprintf("https://api.themoviedb.org/3/tv/%s/season/%d?api_key=%s&language=zh-CN", url.PathEscape(strings.TrimSpace(tmdbID)), season, url.QueryEscape(apiKey))
	cacheKey := fmt.Sprintf("subscription:tv-season:%s:%d", strings.TrimSpace(tmdbID), season)
	var resp subscriptionTMDBSeasonResponse
	if err := s.fetchSubscriptionTMDBJSON(ctx, cacheKey, endpoint, 24*time.Hour, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *SubscriptionService) fetchSubscriptionTMDBJSON(ctx context.Context, cacheKey, requestURL string, ttl time.Duration, out interface{}) error {
	now := time.Now().UTC()

	var cached models.TMDBCache
	if err := db.DB.Where(`"cache_key" = ? AND "expires_at" > ?`, cacheKey, now).First(&cached).Error; err == nil {
		if decodeErr := json.Unmarshal([]byte(cached.CacheValue), out); decodeErr == nil {
			return nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return upstream.SafeUpstreamError(err, "tmdb")
	}

	httpClient := s.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return upstream.SafeUpstreamError(err, "tmdb")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return upstream.SafeUpstreamError(err, "tmdb")
	}
	if resp.StatusCode != http.StatusOK {
		return upstream.SafeUpstreamHTTPError("tmdb", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析 TMDB 响应失败: %w", err)
	}

	cacheRow := models.TMDBCache{
		CacheKey:   cacheKey,
		CacheValue: string(body),
		ExpiresAt:  now.Add(ttl),
	}
	if err := db.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "cache_key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"cache_value": cacheRow.CacheValue,
			"expires_at":  cacheRow.ExpiresAt,
		}),
	}).Create(&cacheRow).Error; err != nil {
		return fmt.Errorf("写入 TMDB 缓存失败: %w", err)
	}

	return nil
}

func parseSubscriptionTMDBAirDate(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return time.Time{}, false
	}
	return normalizeSubscriptionDateUTC(parsed), true
}

// ptrToString 将 *string 安全转为 string；nil 返回空字符串。
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func normalizeSubscriptionDateUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func subscriptionCalendarDateInLocation(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func (s *SubscriptionService) collectIngestUpdates(ctx context.Context, subscriptions []models.Subscription, payload SubscriptionIngestWebhookPayload) (int64, error) {
	if len(subscriptions) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	var updatedCount int64
	for _, subscription := range subscriptions {
		updates := map[string]interface{}{
			"status":      models.SubscriptionIngested,
			"ingested_at": now,
		}
		result := db.DB.WithContext(ctx).Model(&models.Subscription{}).
			Where("id = ? AND status = ?", subscription.ID, models.SubscriptionApproved).
			Updates(updates)
		if result.Error != nil {
			return updatedCount, fmt.Errorf("更新订阅入库状态失败: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			continue
		}

		subscription.Status = models.SubscriptionIngested
		subscription.IngestedAt = &now
		updatedCount += result.RowsAffected
		log.Printf("[Subscription] Webhook 已回写入库 subscriptionId=%s userId=%s type=%s tmdbId=%s season=%d embyItemId=%s",
			subscription.ID, subscription.UserID, subscription.Type, subscription.TmdbID, subscription.Season, strings.TrimSpace(payload.EmbyItemID))
		async.SafeGo("subscription.notifyIngested", func() { s.notifyIngested(subscription) })
	}
	return updatedCount, nil
}

func (s *SubscriptionService) resolveWebhookMatchTMDBIDs(itemType string, payload SubscriptionIngestWebhookPayload) []string {
	matchTMDBIDs := make([]string, 0, 2)
	appendTMDBID := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		for _, existing := range matchTMDBIDs {
			if existing == trimmed {
				return
			}
		}
		matchTMDBIDs = append(matchTMDBIDs, trimmed)
	}

	appendTMDBID(payload.TmdbID)

	if itemType != "episode" {
		return matchTMDBIDs
	}

	seriesID := strings.TrimSpace(payload.SeriesID)
	if seriesID == "" {
		return matchTMDBIDs
	}

	seriesTMDBID, err := resolveSeriesTMDBIDBySeriesID(seriesID)
	if err != nil {
		log.Printf("[Subscription] Webhook 解析剧集主 TMDB ID 失败 seriesId=%s tmdbId=%s err=%v", seriesID, strings.TrimSpace(payload.TmdbID), err)
		return matchTMDBIDs
	}
	if seriesTMDBID != "" && strings.TrimSpace(payload.TmdbID) != seriesTMDBID {
		log.Printf("[Subscription] Webhook 剧集匹配追加主条目 tmdbId=%s seriesTmdbId=%s seriesId=%s", strings.TrimSpace(payload.TmdbID), seriesTMDBID, seriesID)
	}
	appendTMDBID(seriesTMDBID)

	return matchTMDBIDs
}

func resolveSeriesTMDBIDBySeriesID(seriesID string) (string, error) {
	trimmedSeriesID := strings.TrimSpace(seriesID)
	if trimmedSeriesID == "" {
		return "", nil
	}

	if cached, ok := getCachedSeriesTMDBID(trimmedSeriesID); ok {
		return cached, nil
	}

	embyLookup := newSubscriptionEmbyLookup()
	if embyLookup == nil || !embyLookup.IsConfigured() {
		return "", nil
	}

	paths := []struct {
		path   string
		params map[string]string
	}{
		{
			path: "/emby/Items",
			params: map[string]string{
				"Ids":    trimmedSeriesID,
				"Fields": "ProviderIds",
			},
		},
		{
			path: "/emby/Items/" + url.PathEscape(trimmedSeriesID),
			params: map[string]string{
				"Fields": "ProviderIds",
			},
		},
	}

	var lastErr error
	for _, lookup := range paths {
		body, err := embyLookup.GetWithAPIKey(lookup.path, lookup.params)
		if err != nil {
			lastErr = err
			continue
		}

		var resp subscriptionSeriesItemsResponse
		if lookup.path == "/emby/Items" {
			if err := json.Unmarshal(body, &resp); err != nil {
				return "", fmt.Errorf("解析 Emby 剧集主条目失败: %w", err)
			}
			if len(resp.Items) == 0 {
				continue
			}
			resolved := extractProviderID(resp.Items[0].ProviderIDs, "Tmdb")
			cacheSeriesTMDBID(trimmedSeriesID, resolved)
			return resolved, nil
		}

		var item subscriptionSeriesItem
		if err := json.Unmarshal(body, &item); err != nil {
			return "", fmt.Errorf("解析 Emby 剧集主条目失败: %w", err)
		}
		resolved := extractProviderID(item.ProviderIDs, "Tmdb")
		cacheSeriesTMDBID(trimmedSeriesID, resolved)
		return resolved, nil
	}

	if lastErr != nil {
		return "", lastErr
	}
	cacheSeriesTMDBID(trimmedSeriesID, "")
	return "", nil
}

func getCachedSeriesTMDBID(seriesID string) (string, bool) {
	now := subscriptionSeriesTMDBLookupNow()

	subscriptionSeriesTMDBLookupCache.mu.RLock()
	entry, ok := subscriptionSeriesTMDBLookupCache.entries[seriesID]
	subscriptionSeriesTMDBLookupCache.mu.RUnlock()
	if !ok {
		return "", false
	}
	if !entry.expiresAt.After(now) {
		subscriptionSeriesTMDBLookupCache.mu.Lock()
		delete(subscriptionSeriesTMDBLookupCache.entries, seriesID)
		subscriptionSeriesTMDBLookupCache.mu.Unlock()
		return "", false
	}
	return entry.value, true
}

func cacheSeriesTMDBID(seriesID, tmdbID string) {
	subscriptionSeriesTMDBLookupCache.mu.Lock()
	subscriptionSeriesTMDBLookupCache.entries[seriesID] = seriesTMDBLookupCacheEntry{
		value:     tmdbID,
		expiresAt: subscriptionSeriesTMDBLookupNow().Add(subscriptionSeriesTMDBLookupCacheTTL),
	}
	subscriptionSeriesTMDBLookupCache.mu.Unlock()
}

func (s *SubscriptionService) notifyApproved(subscription models.Subscription) {
	user, ok := loadSubscriptionUser(subscription.UserID)
	if !ok || user.TelegramID == nil || s.notifier == nil || !s.notifier.IsConfigured() {
		return
	}
	s.notifier.NotifySubscriptionApproved(notifierint.SubscriptionResultNotification{
		TelegramID:     *user.TelegramID,
		SubscriptionID: subscription.ID,
		UserName:       user.Username,
		Type:           string(subscription.Type),
		Name:           subscription.Name,
		TmdbID:         subscription.TmdbID,
		Season:         subscription.Season,
		PosterPath:     subscription.PosterPath,
		Status:         string(subscription.Status),
		ReviewedAt:     formatNotificationTime(subscription.ReviewedAt),
	})
}

func (s *SubscriptionService) notifyRejected(subscription models.Subscription) {
	user, ok := loadSubscriptionUser(subscription.UserID)
	if !ok || user.TelegramID == nil || s.notifier == nil || !s.notifier.IsConfigured() {
		return
	}
	s.notifier.NotifySubscriptionRejected(notifierint.SubscriptionResultNotification{
		TelegramID:     *user.TelegramID,
		SubscriptionID: subscription.ID,
		UserName:       user.Username,
		Type:           string(subscription.Type),
		Name:           subscription.Name,
		TmdbID:         subscription.TmdbID,
		Season:         subscription.Season,
		PosterPath:     subscription.PosterPath,
		Status:         string(subscription.Status),
		RejectReason:   stringPointerValue(subscription.RejectReason),
		ReviewedAt:     formatNotificationTime(subscription.ReviewedAt),
	})
}

// syncAdminNotifications 将订阅审批最终状态同步到所有已持久化的管理员 Telegram 消息。
func (s *SubscriptionService) syncAdminNotifications(subscription models.Subscription) {
	if s.notifier == nil || !s.notifier.IsConfigured() {
		return
	}

	var notifications []models.SubscriptionAdminNotification
	if err := db.DB.
		Where("subscription_id = ? AND message_id IS NOT NULL AND delivery_status IN ?",
			subscription.ID,
			[]string{
				subscriptionAdminNotificationSent,
				subscriptionAdminNotificationEditFailed,
			},
		).
		Order("created_at ASC").
		Find(&notifications).Error; err != nil {
		log.Printf("[Subscription] 查询管理员审批消息失败 subscriptionId=%s err=%v", subscription.ID, err)
		return
	}
	if len(notifications) == 0 {
		log.Printf("[Subscription] 无可同步管理员审批消息 subscriptionId=%s status=%s", subscription.ID, subscription.Status)
		return
	}

	user, _ := loadSubscriptionUser(subscription.UserID)
	userName := ""
	if user != nil {
		userName = user.Username
	}

	refs := make([]notifierint.SubscriptionAdminNotificationRef, 0, len(notifications))
	for _, notification := range notifications {
		if notification.MessageID == nil || *notification.MessageID <= 0 {
			continue
		}
		refs = append(refs, notifierint.SubscriptionAdminNotificationRef{
			ID:              notification.ID,
			AdminTelegramID: notification.AdminTelegramID,
			ChatID:          notification.ChatID,
			MessageID:       *notification.MessageID,
			HasPhoto:        notification.HasPhoto,
		})
	}
	if len(refs) == 0 {
		return
	}

	results, err := s.notifier.SyncSubscriptionAdminMessages(notifierint.SubscriptionAdminSyncRequest{
		SubscriptionID: subscription.ID,
		UserName:       userName,
		Type:           string(subscription.Type),
		Name:           subscription.Name,
		TmdbID:         subscription.TmdbID,
		Season:         subscription.Season,
		PosterPath:     subscription.PosterPath,
		Note:           subscription.Note,
		Status:         string(subscription.Status),
		RejectReason:   stringPointerValue(subscription.RejectReason),
		ReviewedAt:     formatNotificationTime(subscription.ReviewedAt),
		Notifications:  refs,
	})
	if err != nil {
		log.Printf("[Subscription] 同步管理员审批消息失败 subscriptionId=%s status=%s count=%d err=%v",
			subscription.ID, subscription.Status, len(refs), err)
		return
	}
	s.persistAdminNotificationSyncResults(subscription.ID, results)
}

// persistAdminNotificationSyncResults 写回 Bot 对管理员审批消息的编辑结果。
func (s *SubscriptionService) persistAdminNotificationSyncResults(subscriptionID string, results []notifierint.SubscriptionAdminSyncResult) {
	for _, result := range results {
		id := strings.TrimSpace(result.ID)
		if id == "" {
			continue
		}
		status := strings.TrimSpace(result.DeliveryStatus)
		if status == "" {
			status = subscriptionAdminNotificationSent
		}
		updates := map[string]interface{}{
			"delivery_status": status,
			"failure_reason":  result.FailureReason,
		}
		if err := db.DB.Model(&models.SubscriptionAdminNotification{}).
			Where("id = ? AND subscription_id = ?", id, subscriptionID).
			Updates(updates).Error; err != nil {
			log.Printf("[Subscription] 写回管理员审批消息同步结果失败 subscriptionId=%s notificationId=%s status=%s err=%v",
				subscriptionID, id, status, err)
		}
	}
}

func (s *SubscriptionService) notifyIngested(subscription models.Subscription) {
	user, ok := loadSubscriptionUser(subscription.UserID)
	if !ok || user.TelegramID == nil || s.notifier == nil || !s.notifier.IsConfigured() {
		return
	}
	s.notifier.NotifySubscriptionIngested(notifierint.SubscriptionResultNotification{
		TelegramID:     *user.TelegramID,
		SubscriptionID: subscription.ID,
		UserName:       user.Username,
		Type:           string(subscription.Type),
		Name:           subscription.Name,
		TmdbID:         subscription.TmdbID,
		Season:         subscription.Season,
		PosterPath:     subscription.PosterPath,
		Status:         string(subscription.Status),
		IngestedAt:     formatNotificationTime(subscription.IngestedAt),
	})
}

func loadSubscriptionUser(userID string) (*models.User, bool) {
	var user models.User
	if err := db.DB.Select("id", "username", "telegram_id").Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, false
	}
	return &user, true
}

func formatNotificationTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	loc := configpkg.LoadConfiguredTimezone()
	formatted := value.In(loc).Format(time.RFC3339)
	return &formatted
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
