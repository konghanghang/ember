package subscription

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	moviepilotint "github.com/konghang/ember/backend/internal/integrations/moviepilot"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

// SubscriptionService 订阅服务
type SubscriptionService struct {
	moviepilot *moviepilotint.MoviePilotClient
	notifier   *notifierint.BotNotifier
}

// NewSubscriptionService 创建订阅服务
func NewSubscriptionService() *SubscriptionService {
	return &SubscriptionService{
		moviepilot: moviepilotint.NewMoviePilotClient(),
		notifier:   notifierint.NewBotNotifier(),
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

	var count int64
	if err := db.DB.Model(&models.Subscription{}).
		Where("type = ? AND \"tmdbId\" = ? AND season = ?", req.Type, req.TmdbID, season).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("创建订阅失败: %w", err)
	}
	if count > 0 {
		return nil, ErrSubscriptionDuplicated
	}

	if !req.ConfirmExisting {
		checkResult, err := s.CheckExisting(CheckExistingRequest{
			Type:   req.Type,
			TmdbID: req.TmdbID,
			Season: season,
		})
		if err != nil {
			return nil, err
		}
		if checkResult.ExistsInLibrary || checkResult.DetectionFailed {
			log.Printf("[Subscription] 创建前需要二次确认 userId=%s type=%s tmdbId=%s season=%d exists=%t detectionFailed=%t", userID, req.Type, req.TmdbID, season, checkResult.ExistsInLibrary, checkResult.DetectionFailed)
			return &CreateSubscriptionResult{
				Success:              false,
				ConfirmationRequired: true,
				DetectionFailed:      checkResult.DetectionFailed,
				ExistingSummary:      checkResult.ExistingSummary,
			}, nil
		}
	}

	subscription := &models.Subscription{
		UserID:     userID,
		Type:       req.Type,
		Name:       req.Name,
		TmdbID:     req.TmdbID,
		Season:     season,
		PosterPath: req.PosterPath,
		Note:       req.Note,
		Status:     models.SubscriptionPending,
	}

	if err := db.DB.Create(subscription).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrSubscriptionDuplicated
		}
		return nil, fmt.Errorf("创建订阅失败: %w", err)
	}

	log.Printf("[Subscription] 创建成功 userId=%s subscriptionId=%s type=%s tmdbId=%s season=%d", userID, subscription.ID, req.Type, req.TmdbID, season)
	go s.notifyNewSubscription(subscription.ID, userID, req, season)

	return &CreateSubscriptionResult{Success: true}, nil
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

// ApproveSubscription 批准订阅。
func (s *SubscriptionService) ApproveSubscription(subscriptionID string) error {
	var subscription models.Subscription
	if err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return ErrSubscriptionNotFound
	}
	if subscription.Status != models.SubscriptionPending {
		return ErrSubscriptionHandled
	}

	var mpError *string
	if s.moviepilot.IsConfigured() {
		mpType := "movie"
		if subscription.Type == models.MediaTV {
			mpType = "tv"
		}

		err := s.moviepilot.CreateSubscription(moviepilotint.SubscribeRequest{
			Type:   mpType,
			Name:   subscription.Name,
			TmdbID: subscription.TmdbID,
			Season: subscription.Season,
		})
		if err != nil {
			errMsg := err.Error()
			mpError = &errMsg
			log.Printf("[Subscription] MoviePilot 调用失败 subscriptionId=%s type=%s tmdbId=%s season=%d err=%v", subscription.ID, subscription.Type, subscription.TmdbID, subscription.Season, err)
		}
	} else {
		errMsg := "MoviePilot 未配置"
		mpError = &errMsg
		log.Printf("[Subscription] 跳过 MoviePilot：未配置 subscriptionId=%s type=%s tmdbId=%s season=%d", subscription.ID, subscription.Type, subscription.TmdbID, subscription.Season)
	}

	now := time.Now().UTC()
	subscription.Status = models.SubscriptionApproved
	subscription.ReviewedAt = &now
	subscription.RejectReason = nil
	subscription.MpError = mpError

	if err := db.DB.Save(&subscription).Error; err != nil {
		return fmt.Errorf("更新订阅状态失败: %w", err)
	}

	log.Printf("[Subscription] 审批通过 subscriptionId=%s userId=%s type=%s tmdbId=%s season=%d", subscription.ID, subscription.UserID, subscription.Type, subscription.TmdbID, subscription.Season)
	go s.notifyApproved(subscription)
	return nil
}

// RejectSubscription 拒绝订阅。
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
	subscription.Status = models.SubscriptionRejected
	subscription.ReviewedAt = &now
	subscription.RejectReason = &reason

	if err := db.DB.Save(&subscription).Error; err != nil {
		return fmt.Errorf("更新订阅状态失败: %w", err)
	}

	log.Printf("[Subscription] 审批拒绝 subscriptionId=%s userId=%s type=%s tmdbId=%s season=%d reason=%q", subscription.ID, subscription.UserID, subscription.Type, subscription.TmdbID, subscription.Season, reason)
	go s.notifyRejected(subscription)
	return nil
}

// MarkSubscriptionsIngestedByWebhook 将命中的 APPROVED 订阅回写为 INGESTED。
func (s *SubscriptionService) MarkSubscriptionsIngestedByWebhook(ctx context.Context, payload SubscriptionIngestWebhookPayload) (int64, error) {
	itemType := strings.ToLower(strings.TrimSpace(payload.ItemType))
	tmdbID := strings.TrimSpace(payload.TmdbID)
	if tmdbID == "" {
		log.Printf("[Subscription] Webhook 跳过入库回写：tmdbId 为空 itemType=%s itemName=%q", itemType, strings.TrimSpace(payload.ItemName))
		return 0, nil
	}

	query := db.DB.WithContext(ctx).Model(&models.Subscription{}).Where("status = ? AND \"tmdbId\" = ?", models.SubscriptionApproved, tmdbID)
	switch itemType {
	case "movie":
		query = query.Where("type = ? AND season = 0", models.MediaMovie)
	case "episode":
		if payload.Season <= 0 || payload.Episode <= 0 {
			log.Printf("[Subscription] Webhook 跳过入库回写：季集号无效 tmdbId=%s season=%d episode=%d", tmdbID, payload.Season, payload.Episode)
			return 0, nil
		}
		query = query.Where("type = ? AND (season = 0 OR season = ?)", models.MediaTV, payload.Season)
	default:
		return 0, nil
	}

	var subscriptions []models.Subscription
	if err := query.Find(&subscriptions).Error; err != nil {
		return 0, fmt.Errorf("查询待入库订阅失败: %w", err)
	}
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
		log.Printf("[Subscription] Webhook 已回写入库 subscriptionId=%s userId=%s type=%s tmdbId=%s season=%d embyItemId=%s", subscription.ID, subscription.UserID, subscription.Type, subscription.TmdbID, subscription.Season, strings.TrimSpace(payload.EmbyItemID))
		go s.notifyIngested(subscription)
	}

	return updatedCount, nil
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
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
