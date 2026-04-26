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
	"strings"
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
	moviepilot *moviepilotint.MoviePilotClient
	notifier   *notifierint.BotNotifier
	httpClient *http.Client
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

var newSubscriptionEmbyLookup = func() subscriptionEmbyLookup {
	return embyint.NewEmbyService()
}

var activeSubscriptionStatuses = []models.SubscriptionStatus{
	models.SubscriptionPending,
	models.SubscriptionApproved,
	models.SubscriptionIngested,
}

// NewSubscriptionService 创建订阅服务
func NewSubscriptionService() *SubscriptionService {
	return &SubscriptionService{
		moviepilot: moviepilotint.NewMoviePilotClient(),
		notifier:   notifierint.NewBotNotifier(),
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
		Where("type = ? AND \"tmdbId\" = ? AND season = ? AND status IN ?", mediaType, strings.TrimSpace(tmdbID), season, activeSubscriptionStatuses).
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
	if err := db.DB.Where("id = ?", subscriptionID).First(&original).Error; err != nil {
		return nil, ErrSubscriptionNotFound
	}
	if original.UserID != userID {
		return nil, ErrSubscriptionForbidden
	}
	if original.Status != models.SubscriptionRejected {
		return nil, ErrSubscriptionNotRejected
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
	err := tx.Where("type = ? AND \"tmdbId\" = ? AND season = ? AND status IN ?",
		mediaType, tmdbID, season, activeSubscriptionStatuses).
		Order(`"createdAt" ASC`).
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
			if findErr := db.DB.Where("type = ? AND \"tmdbId\" = ? AND season = ? AND status IN ?",
				mediaType, tmdbID, season, activeSubscriptionStatuses).
				Order(`"createdAt" ASC`).
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

	s.notifier.NotifyNewSubscription(notifierint.SubscriptionNotification{
		ID:         subscriptionID,
		UserName:   username,
		Type:       string(req.Type),
		Name:       req.Name,
		TmdbID:     req.TmdbID,
		Season:     season,
		PosterPath: req.PosterPath,
		Note:       req.Note,
	})
}

// GetUserSubscriptions 获取用户的订阅列表
func (s *SubscriptionService) GetUserSubscriptions(userID string) ([]models.Subscription, error) {
	var subscriptions []models.Subscription
	err := db.DB.
		Where("\"userId\" = ?", userID).
		Order("\"createdAt\" DESC").
		Find(&subscriptions).Error
	if err != nil {
		return nil, fmt.Errorf("查询订阅列表失败: %w", err)
	}
	return subscriptions, nil
}

// GetUserSubscriptionsPaginated 用户订阅分页查询
func (s *SubscriptionService) GetUserSubscriptionsPaginated(userID string, status *models.SubscriptionStatus, page, pageSize int) (*GetAllSubscriptionsResponse, error) {
	offset := (page - 1) * pageSize
	query := db.DB.Model(&models.Subscription{}).Where("\"userId\" = ?", userID)
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("查询订阅总数失败: %w", err)
	}

	var subscriptions []models.Subscription
	if err := query.Order("\"createdAt\" DESC").Offset(offset).Limit(pageSize).Find(&subscriptions).Error; err != nil {
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
	if err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return ErrSubscriptionNotFound
	}
	if subscription.UserID != userID {
		return errors.New("无权删除此订阅")
	}
	if subscription.Status != models.SubscriptionPending {
		return errors.New("只能删除待审核的订阅")
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
	if err := query.Order("\"createdAt\" DESC").Offset(offset).Limit(pageSize).Find(&subscriptions).Error; err != nil {
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
			"status":       models.SubscriptionApproved,
			"reviewedAt":   now,
			"rejectReason": nil,
			"mpError":      nil,
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
	if err := db.DB.Model(&models.Subscription{}).
		Where("id = ?", subscriptionID).
		Update("mpError", mpError).Error; err != nil {
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
			"status":       models.SubscriptionRejected,
			"reviewedAt":   now,
			"rejectReason": reason,
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

// MarkSubscriptionIngestedAsAdmin 允许管理员在校验 Emby 已存在资源后手动收口为已入库。
func (s *SubscriptionService) MarkSubscriptionIngestedAsAdmin(subscriptionID string) error {
	var subscription models.Subscription
	if err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return ErrSubscriptionNotFound
	}
	if subscription.Status != models.SubscriptionApproved {
		return ErrSubscriptionNotApproved
	}

	embyService := embyint.NewEmbyService()
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
			"status":     models.SubscriptionIngested,
			"ingestedAt": now,
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
		Where("status = ? AND \"tmdbId\" IN ? AND type = ? AND season = 0",
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
		Where("status = ? AND \"tmdbId\" IN ? AND type = ? AND season = ?",
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
		Where("status = ? AND \"tmdbId\" IN ? AND type = ? AND season = 0",
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
					"status":         models.SubscriptionIngested,
					"ingestedAt":     now,
					"ingestProgress": progress,
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
			Update("ingestProgress", progress).Error; err != nil {
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
		Select("season", "episode").
		Where(`"tmdbId" = ? AND "airDate" <= ? AND status = ?`, strings.TrimSpace(tmdbID), today, models.MediaGapStatusIgnored).
		Find(&ignored).Error; err != nil {
		return nil, fmt.Errorf("查询整剧已忽略剧集失败: %w", err)
	}

	result := make(wholeShowIgnoredEpisodeSet, len(ignored))
	for _, gap := range ignored {
		result[buildWholeShowEpisodeKey(gap.Season, gap.Episode)] = struct{}{}
	}
	return result, nil
}

func resolveWholeShowSeriesID(tmdbID, seriesID string) (string, error) {
	trimmedSeriesID := strings.TrimSpace(seriesID)
	if trimmedSeriesID != "" {
		return trimmedSeriesID, nil
	}

	embyService := embyint.NewEmbyService()
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
	embyService := embyint.NewEmbyService()
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
	if err := db.DB.Where(`"cacheKey" = ? AND "expiresAt" > ?`, cacheKey, now).First(&cached).Error; err == nil {
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
		Columns: []clause.Column{{Name: "cacheKey"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"cacheValue": cacheRow.CacheValue,
			"expiresAt":  cacheRow.ExpiresAt,
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
			"status":     models.SubscriptionIngested,
			"ingestedAt": now,
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
			return extractProviderID(resp.Items[0].ProviderIDs, "Tmdb"), nil
		}

		var item subscriptionSeriesItem
		if err := json.Unmarshal(body, &item); err != nil {
			return "", fmt.Errorf("解析 Emby 剧集主条目失败: %w", err)
		}
		return extractProviderID(item.ProviderIDs, "Tmdb"), nil
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", nil
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
	if err := db.DB.Select("id", "username", "telegramId").Where("id = ?", userID).First(&user).Error; err != nil {
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
