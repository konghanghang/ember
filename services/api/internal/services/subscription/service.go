package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/konghang/ember/backend/internal/async"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
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

	duplicated, err := hasActiveSubscription(req.Type, req.TmdbID, season)
	if err != nil {
		return nil, fmt.Errorf("创建订阅失败: %w", err)
	}
	if duplicated {
		return nil, ErrSubscriptionDuplicated
	}

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
		Note:       req.Note,
		Status:     models.SubscriptionPending,
	}

	if err := db.DB.Create(subscription).Error; err != nil {
		if isSubscriptionUniqueConflict(err) {
			return nil, ErrSubscriptionDuplicated
		}
		return nil, fmt.Errorf("创建订阅失败: %w", err)
	}

	log.Printf("[Subscription] 创建成功 userId=%s subscriptionId=%s type=%s tmdbId=%s season=%d", userID, subscription.ID, req.Type, req.TmdbID, season)
	async.SafeGo("subscription.notifyNew", func() {
		s.notifyNewSubscription(subscription.ID, userID, req, season)
	})

	return &CreateSubscriptionResult{Success: true}, nil
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

	duplicated, err := hasActiveSubscription(original.Type, original.TmdbID, original.Season)
	if err != nil {
		return nil, fmt.Errorf("重新提交订阅失败: %w", err)
	}
	if duplicated {
		return nil, ErrSubscriptionDuplicated
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
		Note:        &note,
		Status:      models.SubscriptionPending,
		RetryFromID: &retryFromID,
	}

	if err := db.DB.Create(subscription).Error; err != nil {
		if isSubscriptionUniqueConflict(err) {
			return nil, ErrSubscriptionDuplicated
		}
		return nil, fmt.Errorf("重新提交订阅失败: %w", err)
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
	async.SafeGo("subscription.notifyApproved", func() { s.notifyApproved(subscription) })
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
	async.SafeGo("subscription.notifyRejected", func() { s.notifyRejected(subscription) })
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

	log.Printf("[Subscription] 管理员校验后标记已入库 subscriptionId=%s userId=%s type=%s tmdbId=%s season=%d", subscription.ID, subscription.UserID, subscription.Type, subscription.TmdbID, subscription.Season)
	return nil
}

// MarkSubscriptionsIngestedByWebhook 将命中的 APPROVED 订阅回写为 INGESTED。
func (s *SubscriptionService) MarkSubscriptionsIngestedByWebhook(ctx context.Context, payload SubscriptionIngestWebhookPayload) (int64, error) {
	itemType := strings.ToLower(strings.TrimSpace(payload.ItemType))
	matchTMDBIDs := s.resolveWebhookMatchTMDBIDs(itemType, payload)
	if len(matchTMDBIDs) == 0 {
		log.Printf("[Subscription] Webhook 跳过入库回写：缺少可匹配的 tmdbId itemType=%s itemName=%q seriesId=%s", itemType, strings.TrimSpace(payload.ItemName), strings.TrimSpace(payload.SeriesID))
		return 0, nil
	}

	query := db.DB.WithContext(ctx).Model(&models.Subscription{}).Where("status = ? AND \"tmdbId\" IN ?", models.SubscriptionApproved, matchTMDBIDs)
	switch itemType {
	case "movie":
		query = query.Where("type = ? AND season = 0", models.MediaMovie)
	case "episode":
		if payload.Season <= 0 || payload.Episode <= 0 {
			log.Printf("[Subscription] Webhook 跳过入库回写：季集号无效 tmdbIds=%v season=%d episode=%d", matchTMDBIDs, payload.Season, payload.Episode)
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
